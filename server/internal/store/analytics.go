package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// OverviewCounts are totals over a time range.
type OverviewCounts struct {
	Received       int `json:"received"`
	Decided        int `json:"decided"`
	Held           int `json:"held"`
	ByModel        int `json:"by_model"`
	ByPolicy       int `json:"by_policy"`
	ByHuman        int `json:"by_human"`
	ByFailsafe     int `json:"by_failsafe"`
	ByExpiry       int `json:"by_expiry"`
	Approved       int `json:"approved"`
	Denied         int `json:"denied"`
	Expired        int `json:"expired"`
	OptProceed     int `json:"opt_proceed"`
	OptSafer       int `json:"opt_safer"`
	OptStop        int `json:"opt_stop"`
	SaferOffered   int `json:"safer_offered"`
	OutcomeFailed  int `json:"outcome_failed"`
	OutcomeRevert  int `json:"outcome_reversed"`
	OutcomeComplnt int `json:"outcome_complaint"`
	ModelCalls     int `json:"model_calls"`
	ModelErrors    int `json:"model_errors"`
	InputTokens    int `json:"input_tokens"`
	AuditsReviewed int `json:"audits_reviewed"`
	AuditsWrong    int `json:"audits_wrong"`
}

// Percentiles holds latency or duration percentiles.
type Percentiles struct {
	P50 *float64 `json:"p50"`
	P90 *float64 `json:"p90"`
	P95 *float64 `json:"p95"`
	P99 *float64 `json:"p99"`
}

// SeriesPoint is one time bucket of the activity chart.
type SeriesPoint struct {
	Bucket        time.Time `json:"bucket"`
	AutoApproved  int       `json:"auto_approved"`
	HumanApproved int       `json:"human_approved"`
	Denied        int       `json:"denied"`
	Expired       int       `json:"expired"`
	Pending       int       `json:"pending"`
	Held          int       `json:"held"`
}

// LatencyPoint is one time bucket of model latency.
type LatencyPoint struct {
	Bucket time.Time `json:"bucket"`
	P50    *float64  `json:"p50"`
	P95    *float64  `json:"p95"`
	Calls  int       `json:"calls"`
	Errors int       `json:"errors"`
}

// Ranked is a label with a count, for top-N lists.
type Ranked struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
	Held  int    `json:"held,omitempty"`
}

// LiveState is the state of the workspace right now, independent of range.
type LiveState struct {
	Pending           int        `json:"pending"`
	OldestPendingAt   *time.Time `json:"oldest_pending_at"`
	NextDeadline      *time.Time `json:"next_deadline"`
	OpenAudits        int        `json:"open_audits"`
	UnregisteredTools int        `json:"unregistered_tools"`
	ActiveAgents24h   int        `json:"active_agents_24h"`
}

// Overview is everything the dashboard's overview page shows.
type Overview struct {
	From          time.Time      `json:"from"`
	To            time.Time      `json:"to"`
	BucketSeconds int            `json:"bucket_seconds"`
	Counts        OverviewCounts `json:"counts"`
	ReviewSeconds Percentiles    `json:"review_seconds"`
	ModelLatency  Percentiles    `json:"model_latency_ms"`
	AutoLatency   Percentiles    `json:"auto_decision_ms"`
	Series        []SeriesPoint  `json:"series"`
	Latency       []LatencyPoint `json:"latency"`
	TopTools      []Ranked       `json:"top_tools"`
	TopAgents     []Ranked       `json:"top_agents"`
	HoldReasons   []Ranked       `json:"hold_reasons"`
	RiskHistogram []int          `json:"risk_histogram"`
	Flows         []Flow         `json:"flows"`
	Live          LiveState      `json:"live"`
}

// Flow counts the actions that took one path: who decided and what they
// chose. Undecided actions have DecidedBy and Option "pending".
type Flow struct {
	DecidedBy string `json:"decided_by"`
	Option    string `json:"option"`
	Count     int    `json:"count"`
}

// Overview computes dashboard figures for [from, to) in buckets of the given
// size.
func (q *Queries) Overview(ctx context.Context, workspaceID string, from, to time.Time, bucket time.Duration) (*Overview, error) {
	o := &Overview{From: from, To: to, BucketSeconds: int(bucket.Seconds())}
	c := &o.Counts

	err := q.db.QueryRow(ctx, `
		SELECT count(*),
		       count(d.action_id),
		       count(*) FILTER (WHERE a.hold_reasons <> '[]'::jsonb),
		       count(*) FILTER (WHERE d.decided_by = 'model'),
		       count(*) FILTER (WHERE d.decided_by = 'policy'),
		       count(*) FILTER (WHERE d.decided_by = 'human'),
		       count(*) FILTER (WHERE d.decided_by = 'failsafe'),
		       count(*) FILTER (WHERE d.decided_by = 'expiry'),
		       count(*) FILTER (WHERE d.outcome = 'approved'),
		       count(*) FILTER (WHERE d.outcome = 'denied'),
		       count(*) FILTER (WHERE d.outcome = 'expired'),
		       count(*) FILTER (WHERE d.option = 'proceed' AND d.outcome = 'approved'),
		       count(*) FILTER (WHERE d.option = 'proceed_safer' AND d.outcome = 'approved'),
		       count(*) FILTER (WHERE d.option = 'stop'),
		       count(*) FILTER (WHERE a.options @> '[{"key":"proceed_safer","available":true}]')
		FROM actions a LEFT JOIN decisions d ON d.action_id = a.id
		WHERE a.workspace_id = $1 AND a.received_at >= $2 AND a.received_at < $3`, workspaceID, from, to,
	).Scan(&c.Received, &c.Decided, &c.Held, &c.ByModel, &c.ByPolicy, &c.ByHuman, &c.ByFailsafe, &c.ByExpiry,
		&c.Approved, &c.Denied, &c.Expired, &c.OptProceed, &c.OptSafer, &c.OptStop, &c.SaferOffered)
	if err != nil {
		return nil, err
	}

	rows, err := q.db.Query(ctx, `
		SELECT coalesce(d.decided_by, 'pending'), coalesce(d.option, 'pending'), count(*)
		FROM actions a LEFT JOIN decisions d ON d.action_id = a.id
		WHERE a.workspace_id = $1 AND a.received_at >= $2 AND a.received_at < $3
		GROUP BY 1, 2 ORDER BY 3 DESC`, workspaceID, from, to)
	if err != nil {
		return nil, err
	}
	o.Flows, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Flow, error) {
		var f Flow
		return f, r.Scan(&f.DecidedBy, &f.Option, &f.Count)
	})
	if err != nil {
		return nil, err
	}
	if o.Flows == nil {
		o.Flows = []Flow{}
	}

	if err := q.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE o.status = 'failed'),
		       count(*) FILTER (WHERE o.status = 'reversed'),
		       count(*) FILTER (WHERE o.status = 'complaint')
		FROM outcomes o WHERE o.workspace_id = $1 AND o.created_at >= $2 AND o.created_at < $3`, workspaceID, from, to,
	).Scan(&c.OutcomeFailed, &c.OutcomeRevert, &c.OutcomeComplnt); err != nil {
		return nil, err
	}

	if err := q.db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE error <> ''), coalesce(sum(input_tokens), 0),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE error = ''),
		       percentile_cont(0.9) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE error = ''),
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE error = ''),
		       percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE error = '')
		FROM evaluations WHERE workspace_id = $1 AND stage = 'model' AND created_at >= $2 AND created_at < $3`, workspaceID, from, to,
	).Scan(&c.ModelCalls, &c.ModelErrors, &c.InputTokens,
		&o.ModelLatency.P50, &o.ModelLatency.P90, &o.ModelLatency.P95, &o.ModelLatency.P99); err != nil {
		return nil, err
	}

	if err := q.db.QueryRow(ctx, `
		SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY review_seconds),
		       percentile_cont(0.9) WITHIN GROUP (ORDER BY review_seconds),
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY review_seconds),
		       percentile_cont(0.99) WITHIN GROUP (ORDER BY review_seconds)
		FROM decisions WHERE workspace_id = $1 AND decided_by = 'human' AND created_at >= $2 AND created_at < $3`, workspaceID, from, to,
	).Scan(&o.ReviewSeconds.P50, &o.ReviewSeconds.P90, &o.ReviewSeconds.P95, &o.ReviewSeconds.P99); err != nil {
		return nil, err
	}

	if err := q.db.QueryRow(ctx, `
		SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY ms), percentile_cont(0.9) WITHIN GROUP (ORDER BY ms),
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY ms), percentile_cont(0.99) WITHIN GROUP (ORDER BY ms)
		FROM (
		    SELECT extract(epoch FROM a.decided_at - a.received_at) * 1000 AS ms
		    FROM actions a JOIN decisions d ON d.action_id = a.id
		    WHERE a.workspace_id = $1 AND a.received_at >= $2 AND a.received_at < $3
		      AND d.decided_by IN ('model', 'policy', 'failsafe')
		) t`, workspaceID, from, to,
	).Scan(&o.AutoLatency.P50, &o.AutoLatency.P90, &o.AutoLatency.P95, &o.AutoLatency.P99); err != nil {
		return nil, err
	}

	if err := q.db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE verdict <> 'correct')
		FROM audits WHERE workspace_id = $1 AND reviewed_at >= $2 AND reviewed_at < $3`, workspaceID, from, to,
	).Scan(&c.AuditsReviewed, &c.AuditsWrong); err != nil {
		return nil, err
	}

	secs := bucket.Seconds()
	rows, err = q.db.Query(ctx, `
		WITH buckets AS (
		    SELECT generate_series(
		        to_timestamp(floor(extract(epoch FROM $2::timestamptz) / $4) * $4),
		        $3::timestamptz - interval '1 microsecond',
		        make_interval(secs => $4)) AS b
		)
		SELECT b.b,
		       count(a.id) FILTER (WHERE d.outcome = 'approved' AND d.decided_by <> 'human'),
		       count(a.id) FILTER (WHERE d.outcome = 'approved' AND d.decided_by = 'human'),
		       count(a.id) FILTER (WHERE d.outcome = 'denied'),
		       count(a.id) FILTER (WHERE d.outcome = 'expired'),
		       count(a.id) FILTER (WHERE a.status = 'pending_review'),
		       count(a.id) FILTER (WHERE a.hold_reasons <> '[]'::jsonb)
		FROM buckets b
		LEFT JOIN actions a ON a.workspace_id = $1 AND a.received_at >= b.b AND a.received_at < b.b + make_interval(secs => $4)
		                   AND a.received_at >= $2 AND a.received_at < $3
		LEFT JOIN decisions d ON d.action_id = a.id
		GROUP BY b.b ORDER BY b.b`, workspaceID, from, to, secs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p SeriesPoint
		if err := rows.Scan(&p.Bucket, &p.AutoApproved, &p.HumanApproved, &p.Denied, &p.Expired, &p.Pending, &p.Held); err != nil {
			rows.Close()
			return nil, err
		}
		o.Series = append(o.Series, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = q.db.Query(ctx, `
		WITH buckets AS (
		    SELECT generate_series(
		        to_timestamp(floor(extract(epoch FROM $2::timestamptz) / $4) * $4),
		        $3::timestamptz - interval '1 microsecond',
		        make_interval(secs => $4)) AS b
		)
		SELECT b.b,
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY e.latency_ms) FILTER (WHERE e.error = ''),
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY e.latency_ms) FILTER (WHERE e.error = ''),
		       count(e.id), count(e.id) FILTER (WHERE e.error <> '')
		FROM buckets b
		LEFT JOIN evaluations e ON e.workspace_id = $1 AND e.stage = 'model'
		      AND e.created_at >= b.b AND e.created_at < b.b + make_interval(secs => $4)
		      AND e.created_at >= $2 AND e.created_at < $3
		GROUP BY b.b ORDER BY b.b`, workspaceID, from, to, secs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p LatencyPoint
		if err := rows.Scan(&p.Bucket, &p.P50, &p.P95, &p.Calls, &p.Errors); err != nil {
			rows.Close()
			return nil, err
		}
		o.Latency = append(o.Latency, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if o.TopTools, err = q.ranked(ctx, `
		SELECT tool, tool, count(*), count(*) FILTER (WHERE hold_reasons <> '[]'::jsonb)
		FROM actions WHERE workspace_id = $1 AND received_at >= $2 AND received_at < $3
		GROUP BY tool ORDER BY count(*) DESC LIMIT 8`, workspaceID, from, to); err != nil {
		return nil, err
	}
	if o.TopAgents, err = q.ranked(ctx, `
		SELECT a.agent_id, ag.name, count(*), count(*) FILTER (WHERE a.hold_reasons <> '[]'::jsonb)
		FROM actions a JOIN agents ag ON ag.id = a.agent_id
		WHERE a.workspace_id = $1 AND a.received_at >= $2 AND a.received_at < $3
		GROUP BY a.agent_id, ag.name ORDER BY count(*) DESC LIMIT 8`, workspaceID, from, to); err != nil {
		return nil, err
	}
	if o.HoldReasons, err = q.ranked(ctx, `
		SELECT r->>'code', r->>'code', count(*), 0
		FROM actions a, jsonb_array_elements(a.hold_reasons) r
		WHERE a.workspace_id = $1 AND a.received_at >= $2 AND a.received_at < $3
		GROUP BY r->>'code' ORDER BY count(*) DESC LIMIT 10`, workspaceID, from, to); err != nil {
		return nil, err
	}

	o.RiskHistogram = make([]int, 10)
	rows, err = q.db.Query(ctx, `
		SELECT least(floor(risk * 10)::int, 9), count(*)
		FROM decisions WHERE workspace_id = $1 AND risk IS NOT NULL AND created_at >= $2 AND created_at < $3
		GROUP BY 1`, workspaceID, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var bin, n int
		if err := rows.Scan(&bin, &n); err != nil {
			rows.Close()
			return nil, err
		}
		if bin >= 0 && bin < 10 {
			o.RiskHistogram[bin] = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	l := &o.Live
	if err := q.db.QueryRow(ctx, `
		SELECT count(*), min(received_at), min(review_deadline) FROM actions
		WHERE workspace_id = $1 AND status = 'pending_review'`, workspaceID,
	).Scan(&l.Pending, &l.OldestPendingAt, &l.NextDeadline); err != nil {
		return nil, err
	}
	if err := q.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM audits WHERE workspace_id = $1 AND verdict IS NULL),
		       (SELECT count(*) FROM tools WHERE workspace_id = $1 AND NOT registered),
		       (SELECT count(DISTINCT agent_id) FROM actions WHERE workspace_id = $1 AND received_at > now() - interval '24 hours')`,
		workspaceID,
	).Scan(&l.OpenAudits, &l.UnregisteredTools, &l.ActiveAgents24h); err != nil {
		return nil, err
	}
	return o, nil
}

func (q *Queries) ranked(ctx context.Context, sql string, args ...any) ([]Ranked, error) {
	rows, err := q.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ranked{}
	for rows.Next() {
		var r Ranked
		if err := rows.Scan(&r.Key, &r.Label, &r.Count, &r.Held); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CalibrationBin counts labelled decisions in one confidence band.
type CalibrationBin struct {
	Tool    string `json:"tool"`
	Bin     int    `json:"bin"`
	Labeled int    `json:"labeled"`
	Agreed  int    `json:"agreed"`
	// Auto counts automatic decisions in the band, labelled or not.
	Auto int `json:"auto"`
	Held int `json:"held"`
}

// CalibrationBins groups model-judged decisions by tool and by tenths of
// disposition confidence. A decision is labelled when a person either made
// the final call on a held action (agreement: they picked the model's
// choice) or audited an automatic one (agreement: verdict correct).
func (q *Queries) CalibrationBins(ctx context.Context, workspaceID string, from, to time.Time, tool string) ([]CalibrationBin, error) {
	rows, err := q.db.Query(ctx, `
		SELECT a.tool, least(floor(d.model_confidence * 10)::int, 9) AS bin,
		       count(*) FILTER (WHERE d.decided_by = 'human' OR au.verdict IS NOT NULL),
		       count(*) FILTER (WHERE (d.decided_by = 'human' AND d.option = d.model_choice)
		                           OR (d.decided_by <> 'human' AND au.verdict = 'correct')),
		       count(*) FILTER (WHERE d.decided_by = 'model'),
		       count(*) FILTER (WHERE a.hold_reasons <> '[]'::jsonb)
		FROM decisions d
		JOIN actions a ON a.id = d.action_id
		LEFT JOIN audits au ON au.action_id = d.action_id
		WHERE d.workspace_id = $1 AND d.created_at >= $2 AND d.created_at < $3
		  AND d.model_confidence IS NOT NULL AND ($4 = '' OR a.tool = $4)
		GROUP BY 1, 2 ORDER BY 1, 2`, workspaceID, from, to, tool)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CalibrationBin
	for rows.Next() {
		var b CalibrationBin
		if err := rows.Scan(&b.Tool, &b.Bin, &b.Labeled, &b.Agreed, &b.Auto, &b.Held); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
