package decide

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/jev"
)

// Model evaluates System One requests. *jev.Client implements it.
type Model interface {
	Evaluate(ctx context.Context, req jev.Request) (*jev.Result, error)
}

// Stage names for the model calls an assessment makes.
const (
	StageModel   = "model"
	StageRecheck = "model_recheck"
)

const questionDisposition = "disposition"

// Call records one request to the model, successful or not.
type Call struct {
	Stage   string
	Request jev.Request
	Result  *jev.Result
	Err     error
	Latency time.Duration
}

// SignalValue is one signal's answer normalized to a risk between 0 and 1.
type SignalValue struct {
	Type jev.QuestionType `json:"type"`
	// Value is the answer on a 0 to 1 scale before inversion: the yes
	// probability of a noul, the position of a score, the expected risk of a
	// choice.
	Value float64 `json:"value"`
	// Risk is the value as it feeds the composite score, after inversion.
	Risk       float64    `json:"risk"`
	Weight     float64    `json:"weight"`
	Choice     string     `json:"choice,omitempty"`
	Confidence *float64   `json:"confidence,omitempty"`
	Answer     jev.Answer `json:"answer"`
	Levels     []string   `json:"levels,omitempty"`
}

// TrippedGuard is a guard whose line was crossed.
type TrippedGuard struct {
	Signal string          `json:"signal"`
	Value  float64         `json:"value"`
	Forbid []domain.Option `json:"forbid"`
	Reason string          `json:"reason"`
}

// Recheck is the second disposition answer asked with options reversed.
type Recheck struct {
	Choice     domain.Option   `json:"choice"`
	Confidence float64         `json:"confidence"`
	Order      []domain.Option `json:"order"`
	Agrees     bool            `json:"agrees"`
}

// Assessment is everything Veto learned from the model about one call.
type Assessment struct {
	Pack          string                    `json:"pack"`
	PackVersion   int                       `json:"pack_version"`
	Choice        domain.Option             `json:"choice"`
	Confidence    float64                   `json:"confidence"`
	Probabilities map[domain.Option]float64 `json:"probabilities"`
	Order         []domain.Option           `json:"order"`
	Signals       map[string]SignalValue    `json:"signals"`
	Risk          float64                   `json:"risk"`
	Guards        []TrippedGuard            `json:"guards"`
	Recheck       *Recheck                  `json:"recheck,omitempty"`
	Skipped       []string                  `json:"skipped,omitempty"`
}

// Forbidden reports whether a tripped guard forbids the option.
func (a *Assessment) Forbidden(o domain.Option) bool {
	for _, g := range a.Guards {
		if slices.Contains(g.Forbid, o) {
			return true
		}
	}
	return false
}

// Ranking orders the offered options from most to least probable.
func (a *Assessment) Ranking() []domain.Option {
	out := slices.Clone(a.Order)
	sort.SliceStable(out, func(i, j int) bool {
		return a.Probabilities[out[i]] > a.Probabilities[out[j]]
	})
	return out
}

// BuildRequest returns the request Veto sends the model for a call, the
// order in which the options are offered, and the signals left out because
// the state lacks what they require.
func BuildRequest(p *Pack, subj Subject) (jev.Request, []domain.Option, []string) {
	state, present := BuildState(subj)
	order := optionOrder(subj.ActionID, subj.Safer.Available)
	questions := map[string]jev.Question{
		questionDisposition: dispositionQuestion(p, order),
	}
	var skipped []string
	for _, s := range p.Signals {
		if !requirementsMet(s, present) {
			skipped = append(skipped, s.ID)
			continue
		}
		questions[s.ID] = signalQuestion(s)
	}
	return jev.Request{State: state, Questions: questions}, order, skipped
}

// Assess asks the model about one call and reads its answers. With
// doubleCheck, the disposition question is asked a second time in parallel
// with the options in reverse order.
func Assess(ctx context.Context, m Model, p *Pack, subj Subject, doubleCheck bool) (*Assessment, []Call, error) {
	main, order, skipped := BuildRequest(p, subj)
	state := main.State

	calls := []Call{{Stage: StageModel, Request: main}}
	if doubleCheck {
		reversed := slices.Clone(order)
		slices.Reverse(reversed)
		calls = append(calls, Call{Stage: StageRecheck, Request: jev.Request{
			State:     state,
			Questions: map[string]jev.Question{questionDisposition: dispositionQuestion(p, reversed)},
		}})
	}

	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func(c *Call) {
			defer wg.Done()
			start := time.Now()
			c.Result, c.Err = m.Evaluate(ctx, c.Request)
			c.Latency = time.Since(start)
		}(&calls[i])
	}
	wg.Wait()

	if err := calls[0].Err; err != nil {
		return nil, calls, err
	}
	a, err := read(p, calls[0].Result.Response, order)
	if err != nil {
		calls[0].Err = err
		return nil, calls, err
	}
	a.Skipped = skipped
	if doubleCheck {
		rc := calls[1]
		if rc.Err != nil {
			// A failed recheck cannot confirm the first answer, so it counts as
			// disagreement rather than being ignored.
			a.Recheck = &Recheck{Agrees: false, Order: slices.Clone(order)}
			slices.Reverse(a.Recheck.Order)
		} else {
			ans := rc.Result.Response.Answers[questionDisposition]
			rev := slices.Clone(order)
			slices.Reverse(rev)
			a.Recheck = &Recheck{
				Choice:     domain.Option(ans.Choice),
				Confidence: deref(ans.Confidence),
				Order:      rev,
				Agrees:     domain.Option(ans.Choice) == a.Choice,
			}
		}
	}
	return a, calls, nil
}

func read(p *Pack, resp *jev.Response, order []domain.Option) (*Assessment, error) {
	disp, ok := resp.Answers[questionDisposition]
	if !ok {
		return nil, fmt.Errorf("decide: no disposition answer")
	}
	a := &Assessment{
		Pack:          p.Name,
		PackVersion:   p.Version,
		Choice:        domain.Option(disp.Choice),
		Confidence:    deref(disp.Confidence),
		Probabilities: make(map[domain.Option]float64, len(order)),
		Order:         order,
		Signals:       map[string]SignalValue{},
		Guards:        []TrippedGuard{},
	}
	for _, o := range order {
		a.Probabilities[o] = disp.Probabilities[string(o)]
	}

	var weighted, total float64
	for _, s := range p.Signals {
		ans, ok := resp.Answers[s.ID]
		if !ok {
			continue
		}
		sv := normalize(s, ans)
		a.Signals[s.ID] = sv
		if s.Risk != nil {
			weighted += sv.Risk * s.Risk.Weight
			total += s.Risk.Weight
		}
	}
	if total > 0 {
		a.Risk = weighted / total
	}
	for _, g := range p.Guards {
		sv, ok := a.Signals[g.Signal]
		if !ok {
			continue
		}
		if (g.Above != nil && sv.Value > *g.Above) || (g.Below != nil && sv.Value < *g.Below) {
			a.Guards = append(a.Guards, TrippedGuard{Signal: g.Signal, Value: sv.Value, Forbid: g.Forbid, Reason: g.Reason})
		}
	}
	return a, nil
}

func normalize(s Signal, ans jev.Answer) SignalValue {
	sv := SignalValue{Type: s.Type, Answer: ans, Confidence: ans.Confidence}
	switch s.Type {
	case jev.TypeNoul:
		sv.Value = deref(ans.Noul)
	case jev.TypeScore:
		sv.Levels = s.Levels
		if n := len(s.Levels) - 1; n > 0 {
			sv.Value = clamp(deref(ans.Score) / float64(n))
		}
	case jev.TypeChoice:
		sv.Choice = ans.Choice
		for _, c := range s.Choices {
			sv.Value += ans.Probabilities[c.Key] * c.Risk
		}
		sv.Value = clamp(sv.Value)
	}
	sv.Risk = sv.Value
	if s.Risk != nil {
		sv.Weight = s.Risk.Weight
		if s.Risk.Invert {
			sv.Risk = 1 - sv.Value
		}
	}
	return sv
}

func dispositionQuestion(p *Pack, order []domain.Option) jev.Question {
	crit := make(jev.ChoiceCriteria, 0, len(order))
	for _, o := range order {
		crit = append(crit, jev.ChoiceOption{Key: string(o), Description: p.Disposition.Criteria[o]})
	}
	return jev.Question{Type: jev.TypeChoice, Instructions: p.Disposition.Instructions, Criteria: crit}
}

func signalQuestion(s Signal) jev.Question {
	q := jev.Question{Type: s.Type, Instructions: s.Instructions}
	switch s.Type {
	case jev.TypeScore:
		q.Criteria = jev.ScoreCriteria(s.Levels)
	case jev.TypeChoice:
		crit := make(jev.ChoiceCriteria, len(s.Choices))
		for i, c := range s.Choices {
			crit[i] = jev.ChoiceOption{Key: c.Key, Description: c.Description}
		}
		q.Criteria = crit
	case jev.TypeNoul:
		if s.Yes != "" || s.No != "" {
			q.Criteria = &jev.NoulCriteria{True: s.Yes, False: s.No}
		}
	}
	return q
}

func requirementsMet(s Signal, present map[string]bool) bool {
	for _, k := range s.Requires {
		if !present[k] {
			return false
		}
	}
	return true
}

// optionOrder returns the options offered to the model in an order derived
// from the action id. Jev 1.13 can lean toward the first option, so the order
// varies across actions but is reproducible for any one action.
func optionOrder(actionID string, saferAvailable bool) []domain.Option {
	opts := []domain.Option{domain.OptionProceed, domain.OptionStop}
	if saferAvailable {
		opts = []domain.Option{domain.OptionProceed, domain.OptionProceedSafer, domain.OptionStop}
	}
	sum := sha256.Sum256([]byte(actionID))
	r := rand.New(rand.NewPCG(binary.LittleEndian.Uint64(sum[:8]), binary.LittleEndian.Uint64(sum[8:16])))
	r.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
	return opts
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func clamp(f float64) float64 {
	switch {
	case f < 0:
		return 0
	case f > 1:
		return 1
	}
	return f
}
