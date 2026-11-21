package crossrepo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HexmosTech/git-lrc/blastradius"
)

func sampleReport() *blastradius.Report {
	return &blastradius.Report{
		Project: "local-consumer",
		Files: []blastradius.FileReport{{
			Path: "src/ekyc.ts",
			Hunks: []blastradius.HunkReport{{
				FilePath:          "src/ekyc.ts",
				Header:            "@@ -1,1 +1,2 @@",
				NewStart:          100,
				NewLines:          2,
				Weights:           blastradius.DefaultWeights(),
				HygieneMultiplier: 1,
				BlastRadiusRaw:    10,
				ReviewPriorityRaw: 5,
				Symbols: []blastradius.SymbolContribution{{
					QualifiedName:  "local.consumer.EKycService.readAadhaar",
					Name:           "readAadhaar",
					BlastRadiusRaw: 10,
					Signals:        []blastradius.Signal{{Name: "Caller reach", Points: 10, Category: "graph"}},
				}},
			}},
		}},
	}
}

func TestSymbolsDedupesAndSorts(t *testing.T) {
	r := sampleReport()
	r.Files[0].Hunks = append(r.Files[0].Hunks, blastradius.HunkReport{
		FilePath: "src/ekyc.ts",
		NewStart: 200, NewLines: 1,
		Symbols: []blastradius.SymbolContribution{
			{QualifiedName: "local.consumer.EKycService.readAadhaar"},
			{QualifiedName: "local.consumer.A"},
		},
	})
	got := Symbols(r)
	if len(got) != 2 || got[0] != "local.consumer.A" || got[1] != "local.consumer.EKycService.readAadhaar" {
		t.Fatalf("Symbols = %v", got)
	}
}

func TestApplyFoldsImpactsAndRenormalizes(t *testing.T) {
	r := sampleReport()
	resp := &MatchesResponse{
		RelatedRepos: []RelatedRepo{{ID: 3, FullName: "org/integration", Depth: 1}},
		Impacts: map[string][]blastradius.CrossRepoImpact{
			"local.consumer.EKycService.readAadhaar": {{
				RelatedRepo: "root-cache", RelatedRepoID: 3, RelatedRepoName: "org/integration",
				RelatedRepoWebURL: "https://git/org/integration",
				CallSite:          "__route__POST__/v1/e-kyc/read-aadhaar",
				MatchKind:         "exact",
			}},
		},
		Chains: map[string][]blastradius.CrossRepoChain{
			"local.consumer.EKycService.readAadhaar": {{
				Hops: []blastradius.CrossRepoHop{
					{RelatedRepo: "root-cache", RelatedRepoName: "org/integration", CallSite: "r1"},
					{RelatedRepo: "root-common", RelatedRepoName: "org/common", CallSite: "r2"},
				},
			}},
		},
		Diagnostics: []string{"checked 1 related repo"},
	}
	impacted, chained := Apply(r, resp)
	if impacted != 1 || chained != 1 {
		t.Fatalf("Apply counts = %d/%d, want 1/1", impacted, chained)
	}
	hunk := r.Files[0].Hunks[0]
	if len(hunk.ImpactedRelatedRepos) != 1 || hunk.ImpactedRelatedRepos[0].RelatedRepoName != "org/integration" {
		t.Fatalf("hunk impacts = %+v", hunk.ImpactedRelatedRepos)
	}
	if len(hunk.ImpactedRepoChains) != 1 {
		t.Fatalf("hunk chains = %+v", hunk.ImpactedRepoChains)
	}
	if hunk.BlastRadiusNorm != 100 {
		t.Fatalf("hunk norm = %v, want 100", hunk.BlastRadiusNorm)
	}
	if len(r.CrossRepoDiagnostics) != 1 {
		t.Fatalf("diagnostics = %+v", r.CrossRepoDiagnostics)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	r := sampleReport()
	resp := &MatchesResponse{
		Impacts: map[string][]blastradius.CrossRepoImpact{
			"local.consumer.EKycService.readAadhaar": {{RelatedRepo: "p1", RelatedRepoName: "org/a", CallSite: "r1"}},
		},
	}
	Apply(r, resp)
	first := r.Files[0].Hunks[0].BlastRadiusRaw
	Apply(r, resp)
	if got := r.Files[0].Hunks[0].BlastRadiusRaw; got != first {
		t.Fatalf("re-applying the same response changed raw score %v -> %v", first, got)
	}
}

func TestApplyIgnoresUnknownSymbols(t *testing.T) {
	r := sampleReport()
	impacted, _ := Apply(r, &MatchesResponse{
		Impacts: map[string][]blastradius.CrossRepoImpact{"unknown.symbol": {{RelatedRepo: "p1"}}},
	})
	if impacted != 0 {
		t.Fatalf("unknown symbol should not be recorded, got %d", impacted)
	}
	if r.Files[0].Hunks[0].BlastRadiusRaw != 10 {
		t.Fatalf("score changed for an unknown symbol: %v", r.Files[0].Hunks[0].BlastRadiusRaw)
	}
}

func TestBuildSymbolCallsSortedAndSkipsEmpty(t *testing.T) {
	got := BuildSymbolCalls(map[string][]Call{
		"b": {{Path: "/b"}},
		"a": {{Path: "/a"}},
		"c": nil,
	})
	if len(got) != 2 || got[0].Key != "a" || got[1].Key != "b" {
		t.Fatalf("BuildSymbolCalls = %+v", got)
	}
}

func TestFetchMatchesSendsReviewScopedRequest(t *testing.T) {
	var gotPath, gotKey string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-Key")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"review_id": 7,
			"self": {"id": 4, "full_name": "org/consumer", "web_url": "https://git/org/consumer"},
			"related_repos": [{"id": 5, "full_name": "org/integration", "web_url": "https://git/org/integration", "depth": 1, "indexed": true}],
			"impacts": {"sym": [{"RelatedRepo": "p", "RelatedRepoName": "org/integration", "CallSite": "__route__POST__/v1/x"}]},
			"chains": {},
			"diagnostics": ["ok"]
		}`))
	}))
	defer ts.Close()

	resp, err := FetchMatches(context.Background(), ts.URL, "the-key", 7, []SymbolCalls{
		{Key: "sym", Calls: []Call{{Method: "POST", Path: "/v1/x"}}},
	}, DefaultDepth, 5*time.Second)
	if err != nil {
		t.Fatalf("FetchMatches: %v", err)
	}
	if gotPath != "/api/v1/diff-review/7/cross-repo/matches" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "the-key" {
		t.Fatalf("X-API-Key = %q", gotKey)
	}
	if gotBody["depth"].(float64) != DefaultDepth {
		t.Fatalf("depth = %v", gotBody["depth"])
	}
	if len(resp.Impacts["sym"]) != 1 || resp.Impacts["sym"][0].RelatedRepoName != "org/integration" {
		t.Fatalf("impacts = %+v", resp.Impacts)
	}
	if len(resp.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v", resp.Diagnostics)
	}
}
