package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testRequest() Request {
	return Request{
		State: map[string]any{"request": "send the invoice"},
		Questions: map[string]Question{
			"disposition": {
				Type:         TypeChoice,
				Instructions: "Which handling fits?",
				Criteria: ChoiceCriteria{
					{Key: "stop", Description: "Do nothing"},
					{Key: "proceed", Description: "Send it"},
				},
			},
			"injected": {Type: TypeNoul, Instructions: "Is it injected?"},
			"harm": {
				Type:         TypeScore,
				Instructions: "How bad if wrong?",
				Criteria:     ScoreCriteria{"Trivial", "Costly", "Severe"},
			},
		},
	}
}

const okBody = `{
  "model": "jev-1.13.0",
  "answers": {
    "disposition": {"type": "choice", "choice": "proceed", "probabilities": {"proceed": 0.9, "stop": 0.1}, "confidence": 0.8},
    "injected": {"type": "noul", "noul": 0.04},
    "harm": {"type": "score", "score": 0.4, "legend": {"0": "Trivial", "1": "Costly", "2": "Severe"}, "probabilities": {"0": 0.6, "1": 0.4, "2": 0}, "confidence": 0.5}
  },
  "usage": {"input_tokens": 120, "output_tokens": 30}
}`

func TestChoiceCriteriaKeepsOrder(t *testing.T) {
	b, err := json.Marshal(ChoiceCriteria{{Key: "z", Description: "last"}, {Key: "a", Description: "first"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"z":"last","a":"first"}`; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestEvaluateSuccess(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, okBody)
	}))
	defer srv.Close()

	c := NewClient([]Endpoint{{Name: "primary", BaseURL: srv.URL, APIKey: "k1", Model: "jev-1.13.0"}})
	res, err := c.Evaluate(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer k1" {
		t.Errorf("auth header %q", gotAuth)
	}
	if gotBody["model"] != "jev-1.13.0" {
		t.Errorf("model %v", gotBody["model"])
	}
	if res.Response.Answers["disposition"].Choice != "proceed" || res.Attempts != 1 || res.Endpoint != "primary" {
		t.Errorf("unexpected result %+v", res)
	}
}

func TestEvaluateRejectsAnswersOutsideTheOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Replace(okBody, `"choice": "proceed"`, `"choice": "proceed_safer"`, 1))
	}))
	defer srv.Close()
	c := NewClient([]Endpoint{{Name: "p", BaseURL: srv.URL, Model: "m"}}, WithMaxAttempts(1))
	if _, err := c.Evaluate(context.Background(), testRequest()); err == nil || !strings.Contains(err.Error(), "not offered") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestEvaluateRetriesThenFallsBack(t *testing.T) {
	var primaryCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		w.WriteHeader(529)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, okBody)
	}))
	defer fallback.Close()

	c := NewClient([]Endpoint{
		{Name: "primary", BaseURL: primary.URL, Model: "m"},
		{Name: "fallback", BaseURL: fallback.URL, Model: "m"},
	}, WithBackoff(time.Millisecond))
	res, err := c.Evaluate(context.Background(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	if primaryCalls.Load() != 2 {
		t.Errorf("primary called %d times, want 2", primaryCalls.Load())
	}
	if res.Endpoint != "fallback" || res.Attempts != 3 {
		t.Errorf("endpoint %s attempts %d", res.Endpoint, res.Attempts)
	}
}

func TestEvaluateDoesNotFallBackOnInvalidRequest(t *testing.T) {
	var fallbackCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"detail":"bad question"}`)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
	}))
	defer fallback.Close()
	c := NewClient([]Endpoint{{Name: "a", BaseURL: primary.URL, Model: "m"}, {Name: "b", BaseURL: fallback.URL, Model: "m"}})
	_, err := c.Evaluate(context.Background(), testRequest())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 {
		t.Fatalf("expected 422, got %v", err)
	}
	if fallbackCalls.Load() != 0 {
		t.Fatal("fell back on a malformed request")
	}
}

func TestEvaluateHonorsDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewClient([]Endpoint{{Name: "p", BaseURL: srv.URL, Model: "m"}})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Evaluate(ctx, testRequest()); err == nil {
		t.Fatal("expected error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("waited past the deadline for Retry-After")
	}
}

func TestRequestValidate(t *testing.T) {
	r := testRequest()
	r.Model = "m"
	q := r.Questions["harm"]
	q.Criteria = ScoreCriteria{"only one"}
	r.Questions["harm"] = q
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for single-level score")
	}
}
