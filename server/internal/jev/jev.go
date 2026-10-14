// Package jev is a client for TypeSafe's System One API, which serves the Jev
// decision model. A request carries a state and a set of typed questions; the
// response carries one typed answer per question with calibrated
// probabilities. See https://docs.typesafe.ai/api.
package jev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// QuestionType is one of the three System One primitives.
type QuestionType string

const (
	TypeChoice QuestionType = "choice"
	TypeScore  QuestionType = "score"
	TypeNoul   QuestionType = "noul"
)

// Limits documented for jev-1.13 (https://docs.typesafe.ai/api).
const (
	MaxChoiceOptions = 255
	MinScoreLevels   = 2
	MaxScoreLevels   = 10
)

// Question is one judgment to make about the state. Criteria holds
// ChoiceCriteria, ScoreCriteria or *NoulCriteria depending on Type.
type Question struct {
	Type         QuestionType `json:"type"`
	Instructions string       `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

// ChoiceOption is one option of a Choice question.
type ChoiceOption struct {
	Key         string
	Description string
}

// ChoiceCriteria is an ordered list of options. It marshals to a JSON object
// whose keys keep this order, because Jev 1.13 can lean toward the first
// option and Veto controls the order deliberately.
type ChoiceCriteria []ChoiceOption

// MarshalJSON encodes the options as an object in slice order.
func (c ChoiceCriteria) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, o := range c {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(o.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(o.Description)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Keys returns the option keys in order.
func (c ChoiceCriteria) Keys() []string {
	keys := make([]string, len(c))
	for i, o := range c {
		keys[i] = o.Key
	}
	return keys
}

// ScoreCriteria is the ordered list of level descriptions of a Score.
type ScoreCriteria []string

// NoulCriteria optionally describes what yes and no mean.
type NoulCriteria struct {
	True  string `json:"true,omitempty"`
	False string `json:"false,omitempty"`
}

// Request is the body of POST /v1/systemone.
type Request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// Validate checks the request against the documented API limits so mistakes
// are caught before a network round trip.
func (r Request) Validate() error {
	if r.Model == "" {
		return errors.New("jev: model is required")
	}
	if len(r.Questions) == 0 {
		return errors.New("jev: at least one question is required")
	}
	for id, q := range r.Questions {
		if q.Instructions == "" {
			return fmt.Errorf("jev: question %q has no instructions", id)
		}
		switch q.Type {
		case TypeChoice:
			c, ok := q.Criteria.(ChoiceCriteria)
			if !ok || len(c) == 0 || len(c) > MaxChoiceOptions {
				return fmt.Errorf("jev: choice %q needs 1 to %d options", id, MaxChoiceOptions)
			}
			seen := make(map[string]bool, len(c))
			for _, o := range c {
				if seen[o.Key] {
					return fmt.Errorf("jev: choice %q repeats option %q", id, o.Key)
				}
				seen[o.Key] = true
			}
		case TypeScore:
			c, ok := q.Criteria.(ScoreCriteria)
			if !ok || len(c) < MinScoreLevels || len(c) > MaxScoreLevels {
				return fmt.Errorf("jev: score %q needs %d to %d levels", id, MinScoreLevels, MaxScoreLevels)
			}
		case TypeNoul:
			if q.Criteria != nil {
				if _, ok := q.Criteria.(*NoulCriteria); !ok {
					return fmt.Errorf("jev: noul %q has criteria of the wrong type", id)
				}
			}
		default:
			return fmt.Errorf("jev: question %q has unknown type %q", id, q.Type)
		}
	}
	return nil
}

// Answer is one typed answer. Which fields are set depends on Type.
type Answer struct {
	Type          QuestionType       `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Usage reports token counts for a request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is the body returned by POST /v1/systemone.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Validate checks that the response answers every question with the right
// type and only with options the question offered. A response that fails
// this check is treated as a model failure, never acted on.
func (r *Response) Validate(req Request) error {
	for id, q := range req.Questions {
		a, ok := r.Answers[id]
		if !ok {
			return fmt.Errorf("jev: no answer for question %q", id)
		}
		if a.Type != q.Type {
			return fmt.Errorf("jev: answer %q has type %q, want %q", id, a.Type, q.Type)
		}
		switch q.Type {
		case TypeChoice:
			crit := q.Criteria.(ChoiceCriteria)
			valid := make(map[string]bool, len(crit))
			for _, o := range crit {
				valid[o.Key] = true
			}
			if !valid[a.Choice] {
				return fmt.Errorf("jev: answer %q chose %q, which was not offered", id, a.Choice)
			}
			for k := range a.Probabilities {
				if !valid[k] {
					return fmt.Errorf("jev: answer %q has probability for unknown option %q", id, k)
				}
			}
			if a.Confidence == nil {
				return fmt.Errorf("jev: choice answer %q has no confidence", id)
			}
		case TypeScore:
			if a.Score == nil || a.Confidence == nil {
				return fmt.Errorf("jev: score answer %q is incomplete", id)
			}
			levels := float64(len(q.Criteria.(ScoreCriteria)) - 1)
			if *a.Score < 0 || *a.Score > levels {
				return fmt.Errorf("jev: score answer %q is %v, outside 0..%v", id, *a.Score, levels)
			}
		case TypeNoul:
			if a.Noul == nil || *a.Noul < 0 || *a.Noul > 1 {
				return fmt.Errorf("jev: noul answer %q is missing or outside 0..1", id)
			}
		}
	}
	return nil
}
