package appcore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDatabaseReports_MarksSourceAndParsesReports(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/review-coverage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing/wrong X-API-Key header: %q", r.Header.Get("X-API-Key"))
		}
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"commits": ["deadbeef"],
			"reports": [
				{"ref": "deadbeef", "review_id": "42", "trigger_type": "mcp", "status": "completed", "user_email": "a@b.com"}
			]
		}`))
	}))
	defer ts.Close()

	reports, err := fetchDatabaseReports(ts.URL, "test-key", []string{"deadbeef"})
	if err != nil {
		t.Fatalf("fetchDatabaseReports returned error: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d: %+v", len(reports), reports)
	}
	if reports[0].Source != "database" {
		t.Errorf("expected source to be forced to 'database', got %q", reports[0].Source)
	}
	if reports[0].ReviewID != "42" || reports[0].TriggerType != "mcp" {
		t.Errorf("unexpected report fields: %+v", reports[0])
	}
	if len(gotBody) == 0 {
		t.Error("expected a request body to be sent")
	}
}

func TestFetchDatabaseReports_NonOKStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer ts.Close()

	_, err := fetchDatabaseReports(ts.URL, "bad-key", []string{"deadbeef"})
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestFetchDatabaseReports_ExpandsRangeViaRevList(t *testing.T) {
	first, second := initTestRepoWithCommits(t)
	rangeRef := first + ".." + second

	var gotCommits []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var parsed struct {
			Commits []string `json:"commits"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		gotCommits = parsed.Commits

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"commits": [], "reports": []}`))
	}))
	defer ts.Close()

	_, err := fetchDatabaseReports(ts.URL, "test-key", []string{rangeRef})
	if err != nil {
		t.Fatalf("fetchDatabaseReports returned error: %v", err)
	}

	foundRange := false
	foundCommit := false
	for _, c := range gotCommits {
		if c == rangeRef {
			foundRange = true
		}
		if c == second {
			foundCommit = true
		}
	}
	if !foundRange {
		t.Errorf("expected literal range ref %q in query set, got %v", rangeRef, gotCommits)
	}
	if !foundCommit {
		t.Errorf("expected expanded commit %q in query set, got %v", second, gotCommits)
	}
}

func TestCollectGitLogReports_SkipsCommitsWithNoTrailer(t *testing.T) {
	first, second := initTestRepoWithCommits(t)

	reports, err := collectGitLogReports([]string{first, second})
	if err != nil {
		t.Fatalf("collectGitLogReports returned error: %v", err)
	}
	// Neither fixture commit carries a LiveReview trailer, so there should
	// be no git_log reports at all -- confirms "none" actions are filtered.
	if len(reports) != 0 {
		t.Errorf("expected no reports (no trailers present), got %+v", reports)
	}
}
