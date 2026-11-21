package blastradius

import (
	"encoding/json"
	"strings"
	"testing"
)

func crossRepoTestReport() *Report {
	return &Report{
		Project: "acme/consumer",
		Files: []FileReport{
			{
				Path: "src/service.ts",
				Hunks: []HunkReport{
					{
						FilePath: "src/service.ts",
						Header:   "@@ -1,1 +1,2 @@",
						NewStart: 10,
						NewLines: 2,
						Weights:  DefaultWeights(),
						// ScoreHunks always sets this; a hand-built fixture has
						// to as well or Renormalize would treat 0 as 1.0.
						HygieneMultiplier: 1,
						BlastRadiusRaw:    20,
						ReviewPriorityRaw: 10,
						Symbols: []SymbolContribution{
							{
								QualifiedName:  "acme.consumer.EKycService.readAadhaar",
								Name:           "readAadhaar",
								BlastRadiusRaw: 20,
								Signals: []Signal{
									{Name: "Caller reach", Points: 20, Category: "graph"},
								},
							},
						},
					},
					{
						FilePath:          "src/service.ts",
						Header:            "@@ -20,1 +20,2 @@",
						NewStart:          20,
						NewLines:          2,
						Weights:           DefaultWeights(),
						HygieneMultiplier: 1,
						BlastRadiusRaw:    4,
						ReviewPriorityRaw: 4,
						Symbols: []SymbolContribution{
							{QualifiedName: "acme.consumer.EKycService.helper", Name: "helper", BlastRadiusRaw: 4},
						},
					},
				},
			},
		},
	}
}

func TestRecordCrossRepoImpact_AddsCappedSignalAndRenormalizes(t *testing.T) {
	r := crossRepoTestReport()
	impacts := []CrossRepoImpact{
		{RelatedRepo: "root-cache-integration", RelatedRepoName: "acme/integration", RelatedRepoID: 7, RelatedRepoWebURL: "https://git/acme/integration", CallSite: "__route__POST__/v1/read"},
	}
	if !RecordCrossRepoImpact(r, "src/service.ts", 10, 2, "acme.consumer.EKycService.readAadhaar", impacts) {
		t.Fatal("RecordCrossRepoImpact did not find the symbol")
	}
	Renormalize(r)

	hunk := r.Files[0].Hunks[0]
	if got, want := hunk.BlastRadiusRaw, 28.0; got != want {
		t.Fatalf("BlastRadiusRaw = %v, want %v", got, want)
	}
	if hunk.MaxBlastRadiusRaw != 28 {
		t.Fatalf("MaxBlastRadiusRaw = %v, want 28", hunk.MaxBlastRadiusRaw)
	}
	if hunk.BlastRadiusNorm != 100 {
		t.Fatalf("BlastRadiusNorm = %v, want 100", hunk.BlastRadiusNorm)
	}
	// The other hunk's norm must be recomputed against the new max.
	if got, want := r.Files[0].Hunks[1].BlastRadiusNorm, 4.0/28.0*100; got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("second hunk norm = %v, want %v", got, want)
	}
	if len(hunk.ImpactedRelatedRepos) != 1 || hunk.ImpactedRelatedRepos[0].RelatedRepoName != "acme/integration" {
		t.Fatalf("hunk rollup = %+v", hunk.ImpactedRelatedRepos)
	}
	sym := hunk.Symbols[0]
	if len(sym.Signals) != 2 {
		t.Fatalf("signals = %+v, want caller + cross-repo", sym.Signals)
	}
	cross := sym.Signals[1]
	if cross.Name != CrossRepoSignalName || cross.Points != 8 || cross.Category != "graph" {
		t.Fatalf("cross-repo signal = %+v", cross)
	}
	if !strings.Contains(cross.Detail, "Calls into acme/integration") {
		t.Fatalf("detail = %q, want outbound wording", cross.Detail)
	}
}

func TestRecordCrossRepoImpact_IsIdempotent(t *testing.T) {
	r := crossRepoTestReport()
	sym := "acme.consumer.EKycService.readAadhaar"
	first := []CrossRepoImpact{{RelatedRepo: "p1", RelatedRepoName: "acme/a", CallSite: "r1"}}
	second := []CrossRepoImpact{{RelatedRepo: "p2", RelatedRepoName: "acme/b", CallSite: "r2"}}

	if !RecordCrossRepoImpact(r, "src/service.ts", 10, 2, sym, first) {
		t.Fatal("first record failed")
	}
	Renormalize(r)
	rawAfterFirst := r.Files[0].Hunks[0].BlastRadiusRaw
	if !RecordCrossRepoImpact(r, "src/service.ts", 10, 2, sym, second) {
		t.Fatal("second record failed")
	}
	Renormalize(r)
	if got := r.Files[0].Hunks[0].BlastRadiusRaw; got != rawAfterFirst {
		t.Fatalf("raw after replacing = %v, want %v (must not stack)", got, rawAfterFirst)
	}
	hunk := r.Files[0].Hunks[0]
	if len(hunk.ImpactedRelatedRepos) != 1 || hunk.ImpactedRelatedRepos[0].RelatedRepoName != "acme/b" {
		t.Fatalf("rollup = %+v, want only the replacement", hunk.ImpactedRelatedRepos)
	}
}

func TestRecordCrossRepoChains_DedupesHunkRollup(t *testing.T) {
	r := crossRepoTestReport()
	chain := CrossRepoChain{Hops: []CrossRepoHop{
		{RelatedRepo: "p1", RelatedRepoName: "acme/a", CallSite: "r1", Handler: "h1"},
		{RelatedRepo: "p2", RelatedRepoName: "acme/b", CallSite: "r2", Handler: "h2"},
	}}
	if !RecordCrossRepoChains(r, "src/service.ts", 10, 2, "acme.consumer.EKycService.readAadhaar", []CrossRepoChain{chain, chain}) {
		t.Fatal("RecordCrossRepoChains did not find the symbol")
	}
	Renormalize(r)
	hunk := r.Files[0].Hunks[0]
	if len(hunk.ImpactedRepoChains) != 1 {
		t.Fatalf("hunk chains = %+v, want deduped by signature", hunk.ImpactedRepoChains)
	}
	sym := hunk.Symbols[0]
	found := false
	for _, s := range sym.Signals {
		if s.Name == CrossRepoChainSignalName {
			found = true
			if s.Points != 4 {
				t.Fatalf("chain bonus = %v, want 4 for one extra hop", s.Points)
			}
		}
	}
	if !found {
		t.Fatal("chain signal missing")
	}
}

func TestRenormalizePreservesARealZeroHygieneMultiplier(t *testing.T) {
	r := crossRepoTestReport()
	r.Files[0].Hunks[0].HygieneMultiplier = 0
	r.Files[0].Hunks[0].BlastRadiusRaw = 50
	Renormalize(r)
	if got := r.Files[0].Hunks[0].Combined; got != 0 {
		t.Fatalf("Combined = %v, want 0 for a fully hygiene-suppressed hunk", got)
	}
}

func TestCrossRepoJSONFieldNames(t *testing.T) {
	payload, err := json.Marshal(CrossRepoImpact{
		RelatedRepo:       "proj",
		RelatedRepoID:     3,
		RelatedRepoName:   "acme/integration",
		RelatedRepoWebURL: "https://git/acme/integration",
		CallSite:          "__route__POST__/v1/x",
		EdgeType:          "CROSS_HTTP_CALLS",
		Confidence:        0.95,
		MatchKind:         "exact",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"RelatedRepo", "RelatedRepoID", "RelatedRepoName", "RelatedRepoWebURL", "CallSite", "EdgeType", "Confidence", "MatchKind"} {
		if !strings.Contains(string(payload), `"`+field+`"`) {
			t.Fatalf("marshalled impact %s missing field %q", payload, field)
		}
	}
}
