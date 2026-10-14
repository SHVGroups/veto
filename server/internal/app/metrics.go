package app

import (
	"context"
	"net/http"

	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/metrics"
)

// Metrics are the counters and histograms the app layer updates.
type Metrics struct {
	Actions         *metrics.CounterVec
	DecisionSeconds *metrics.HistogramVec
	ModelRequests   *metrics.CounterVec
	ModelSeconds    *metrics.HistogramVec
	Reviews         *metrics.CounterVec
	Outcomes        *metrics.CounterVec
	Grants          *metrics.CounterVec
	Executions      *metrics.CounterVec
}

// NewMetrics registers the app's metrics. A nil registry gives working
// metrics that are not exported, which keeps tests simple.
func NewMetrics(r *metrics.Registry) *Metrics {
	if r == nil {
		r = metrics.NewRegistry()
	}
	return &Metrics{
		Actions: r.Counter("veto_actions_total",
			"Actions received, by source, route (auto or human) and resolution.", "source", "route", "resolution"),
		DecisionSeconds: r.Histogram("veto_decision_seconds",
			"Time from receiving an action to deciding or holding it.", metrics.DefaultLatencyBuckets, "route"),
		ModelRequests: r.Counter("veto_model_requests_total",
			"Decision model requests, by stage and result.", "stage", "result"),
		ModelSeconds: r.Histogram("veto_model_request_seconds",
			"Decision model request latency, including retries.", metrics.DefaultLatencyBuckets, "stage"),
		Reviews: r.Counter("veto_reviews_total",
			"Held actions decided by a person, by option and channel.", "option", "channel"),
		Outcomes: r.Counter("veto_outcomes_total",
			"Outcome reports, by status.", "status"),
		Grants: r.Counter("veto_grant_verifications_total",
			"Grant verifications, by result.", "result"),
		Executions: r.Counter("veto_connection_executions_total",
			"Calls executed through connections, by result.", "result"),
	}
}

// ConnectorExecutor performs calls through connections.
type ConnectorExecutor interface {
	Do(ctx context.Context, conn connector.Connection, r connector.Request) (*connector.Response, error)
}

// HTTPConnector adapts connector.Executor to ConnectorExecutor.
func HTTPConnector(c *http.Client) ConnectorExecutor { return connector.NewExecutor(c) }
