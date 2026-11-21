package reviewapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubmitReview_ParsesEnvelopeContract(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/diff-review" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"review_id":"r-123",
			"status":"processing",
			"friendly_name":"quiet-river",
			"envelope":{
				"plan_code":"free_30k",
				"usage_percent":12,
				"loc_used_month":12000,
				"loc_limit_month":30000,
				"loc_remaining_month":18000,
				"blocked":false,
				"trial_readonly":false,
				"ai_execution_mode":"byok_required",
				"ai_execution_source":"connector"
			}
		}`))
	}))
	defer ts.Close()

	encoded := base64.StdEncoding.EncodeToString([]byte("diff"))
	resp, err := SubmitReview(ts.URL, "test-key", encoded, "repo", false)
	if err != nil {
		t.Fatalf("SubmitReview returned error: %v", err)
	}
	if resp.ReviewID != "r-123" {
		t.Fatalf("unexpected review id: %s", resp.ReviewID)
	}
	if resp.Envelope == nil {
		t.Fatal("expected envelope in create response")
	}
	if resp.Envelope.PlanCode != "free_30k" {
		t.Fatalf("unexpected plan code: %s", resp.Envelope.PlanCode)
	}
	if resp.Envelope.UsagePercent == nil || *resp.Envelope.UsagePercent != 12 {
		t.Fatalf("unexpected usage percent: %#v", resp.Envelope.UsagePercent)
	}
	if resp.Envelope.AIExecutionMode != "byok_required" {
		t.Fatalf("unexpected ai execution mode: %s", resp.Envelope.AIExecutionMode)
	}
}

func TestPollReview_ParsesEnvelopeContract(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/diff-review/r-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"completed",
			"summary":"ok",
			"files":[],
			"envelope":{
				"plan_code":"team_32usd",
				"usage_percent":22,
				"loc_used_month":22000,
				"loc_limit_month":100000,
				"loc_remaining_month":78000,
				"blocked":false,
				"trial_readonly":false,
				"operation_type":"diff_review",
				"ai_execution_mode":"hosted_auto",
				"ai_execution_source":"platform"
			}
		}`))
	}))
	defer ts.Close()

	cancel := make(chan struct{})
	resp, err := PollReview(ts.URL, "test-key", "r-123", 10*time.Millisecond, 500*time.Millisecond, false, cancel, nil)
	if err != nil {
		t.Fatalf("PollReview returned error: %v", err)
	}
	if resp == nil || resp.Envelope == nil {
		t.Fatal("expected envelope in poll response")
	}
	if resp.Envelope.OperationType != "diff_review" {
		t.Fatalf("unexpected operation type: %s", resp.Envelope.OperationType)
	}
	if resp.Envelope.AIExecutionMode != "hosted_auto" {
		t.Fatalf("unexpected ai execution mode: %s", resp.Envelope.AIExecutionMode)
	}
}

func TestSubmitReviewWithOptionsSendsRepoRemoteURL(t *testing.T) {
	var gotRepoRemoteURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			RepoName      string `json:"repo_name"`
			RepoRemoteURL string `json:"repo_remote_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotRepoRemoteURL = payload.RepoRemoteURL
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"review_id":"r-1","status":"processing"}`))
	}))
	defer ts.Close()

	_, err := SubmitReviewWithOptions(SubmitOptions{
		APIURL:        ts.URL,
		APIKey:        "key",
		Base64Diff:    base64.StdEncoding.EncodeToString([]byte("diff")),
		RepoName:      "consumer",
		RepoRemoteURL: "git@git.apps.hexmos.com:framework-examples/typescript/nestjs/blive-microservice-2-consumer.git",
	})
	if err != nil {
		t.Fatalf("SubmitReviewWithOptions: %v", err)
	}
	if gotRepoRemoteURL != "git@git.apps.hexmos.com:framework-examples/typescript/nestjs/blive-microservice-2-consumer.git" {
		t.Fatalf("repo_remote_url = %q", gotRepoRemoteURL)
	}
}
