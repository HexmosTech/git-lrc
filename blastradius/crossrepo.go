package blastradius

import (
	"fmt"
	"math"
)

// Cross-repo enrichment for locally-scored reports.
//
// Ported from LiveReview's internal/blastradius/crossrepo.go, which built on
// the report shape this package already produces. Keeping the field names and
// JSON tags identical means a git-lrc report enriched here can be uploaded
// through the existing artifact channel and rendered by the hosted review UI
// with no server-side translation.
//
// The CLI does not index related repositories. It asks LiveReview for the
// impacts of its own touched symbols' outbound calls (see the cross-repo
// matches endpoint in docs/multi-repo-blast-radius-plan.md), then folds those
// impacts into the report with the functions below.

// CrossRepoSignalName is the Signal.Name the direct cross-repo impact bonus is
// recorded under.
const CrossRepoSignalName = "Cross-repo impact"

// CrossRepoChainSignalName labels the small, capped bonus for reaching a repo
// beyond the direct neighbours of the reviewed repo.
const CrossRepoChainSignalName = "Cross-repo chain"

const (
	crossRepoChainBonusPerHop = 4.0
	crossRepoChainMaxBonus    = 8.0
	crossRepoBonusPerImpact   = 8.0
	crossRepoMaxBonus         = 20.0
)

// CrossRepoImpact is one outbound call from a symbol this hunk touches to a
// route/handler declared by a related repository.
type CrossRepoImpact struct {
	// RelatedRepo is the engine project id of the target. It is kept for
	// backwards compatibility with older artifacts; display code must prefer
	// RelatedRepoName.
	RelatedRepo string `json:"RelatedRepo"`
	// RelatedRepoID/RelatedRepoName/RelatedRepoWebURL come from LiveReview's
	// repositories row for the target, so the UI can show "org/service" and
	// link to the provider instead of rendering an index-cache path.
	RelatedRepoID     int64  `json:"RelatedRepoID,omitempty"`
	RelatedRepoName   string `json:"RelatedRepoName,omitempty"`
	RelatedRepoWebURL string `json:"RelatedRepoWebURL,omitempty"`
	// CallSite is the matched route's qualified name, e.g.
	// __route__POST__/v1/e-kyc/read-aadhaar.
	CallSite   string  `json:"CallSite"`
	EdgeType   string  `json:"EdgeType"`
	Confidence float64 `json:"Confidence,omitempty"`
	// MatchKind is "exact", "version-alias" (a /v1 call matched an unversioned
	// route, or vice versa), or empty for older artifacts.
	MatchKind string `json:"MatchKind,omitempty"`
}

// CrossRepoHop is one link in a multi-repo impact chain. Hops[0] is the first
// related repo reached from the reviewed repo; Hops[1] is the repo that repo
// calls, and so on.
type CrossRepoHop struct {
	RelatedRepo       string `json:"RelatedRepo"`
	RelatedRepoID     int64  `json:"RelatedRepoID,omitempty"`
	RelatedRepoName   string `json:"RelatedRepoName,omitempty"`
	RelatedRepoWebURL string `json:"RelatedRepoWebURL,omitempty"`
	CallSite          string `json:"CallSite"`
	Handler           string `json:"Handler,omitempty"`
	EdgeType          string `json:"EdgeType"`
}

// CrossRepoChain is a path of cross-repo calls reached from one touched
// symbol, e.g. reviewed repo -> service B -> service C. Informational: the
// direct hop already contributes the main Cross-repo impact signal.
type CrossRepoChain struct {
	Hops []CrossRepoHop `json:"Hops"`
}

// RecordCrossRepoImpact attaches impacts to the symbol named symbolQN inside
// the hunk identified by (filePath, newStart, newLines), adds a capped
// "graph"-category Signal so it participates in Combined/Tier via the normal
// scoring path, and returns whether a matching hunk+symbol was found. The
// caller must call Renormalize on the whole report afterward (not here, so a
// batch of calls only renormalizes once).
func RecordCrossRepoImpact(report *Report, filePath string, newStart, newLines int, symbolQN string, impacts []CrossRepoImpact) bool {
	if report == nil || len(impacts) == 0 {
		return false
	}
	for fi := range report.Files {
		file := &report.Files[fi]
		if file.Path != filePath {
			continue
		}
		for hi := range file.Hunks {
			hunk := &file.Hunks[hi]
			if hunk.NewStart != newStart || hunk.NewLines != newLines {
				continue
			}
			for si := range hunk.Symbols {
				sym := &hunk.Symbols[si]
				if sym.QualifiedName != symbolQN {
					continue
				}
				// Idempotent, not additive: a re-run replaces the prior
				// cross-repo bonus instead of stacking a second one.
				if prior := removeSignalByName(sym, CrossRepoSignalName); prior != 0 {
					sym.BlastRadiusRaw -= prior
					hunk.BlastRadiusRaw -= prior
				}
				bonus := math.Min(float64(len(impacts))*crossRepoBonusPerImpact, crossRepoMaxBonus)
				sym.ImpactedRelatedRepos = impacts
				sym.Signals = append(sym.Signals, Signal{
					Name:     CrossRepoSignalName,
					Detail:   crossRepoDetail(impacts),
					Points:   bonus,
					Category: "graph",
				})
				sym.BlastRadiusRaw += bonus
				hunk.BlastRadiusRaw += bonus
				return true
			}
		}
	}
	return false
}

// RecordCrossRepoChains attaches multi-hop chains to the symbol named
// symbolQN inside (filePath, newStart, newLines), adds a small capped
// "graph"-category Signal for reach beyond direct neighbours, and returns
// whether the symbol was found. Idempotent, like RecordCrossRepoImpact.
func RecordCrossRepoChains(report *Report, filePath string, newStart, newLines int, symbolQN string, chains []CrossRepoChain) bool {
	if report == nil || len(chains) == 0 {
		return false
	}
	for fi := range report.Files {
		file := &report.Files[fi]
		if file.Path != filePath {
			continue
		}
		for hi := range file.Hunks {
			hunk := &file.Hunks[hi]
			if hunk.NewStart != newStart || hunk.NewLines != newLines {
				continue
			}
			for si := range hunk.Symbols {
				sym := &hunk.Symbols[si]
				if sym.QualifiedName != symbolQN {
					continue
				}
				if prior := removeSignalByName(sym, CrossRepoChainSignalName); prior != 0 {
					sym.BlastRadiusRaw -= prior
					hunk.BlastRadiusRaw -= prior
				}
				maxExtraHops := 0
				for _, c := range chains {
					if extra := len(c.Hops) - 1; extra > maxExtraHops {
						maxExtraHops = extra
					}
				}
				sym.ImpactedRepoChains = chains
				if maxExtraHops > 0 {
					bonus := math.Min(float64(maxExtraHops)*crossRepoChainBonusPerHop, crossRepoChainMaxBonus)
					sym.Signals = append(sym.Signals, Signal{
						Name:     CrossRepoChainSignalName,
						Detail:   crossRepoChainDetail(chains),
						Points:   bonus,
						Category: "graph",
					})
					sym.BlastRadiusRaw += bonus
					hunk.BlastRadiusRaw += bonus
				}
				return true
			}
		}
	}
	return false
}

func removeSignalByName(sym *SymbolContribution, name string) float64 {
	var prior float64
	kept := sym.Signals[:0]
	for _, s := range sym.Signals {
		if s.Name == name {
			prior += s.Points
			continue
		}
		kept = append(kept, s)
	}
	sym.Signals = kept
	return prior
}

// CrossRepoDisplayName prefers the LiveReview repository name over the
// engine's project id, so older artifacts (which only have RelatedRepo) still
// render something and new ones show "org/service".
func CrossRepoDisplayName(im CrossRepoImpact) string {
	if im.RelatedRepoName != "" {
		return im.RelatedRepoName
	}
	return im.RelatedRepo
}

func crossRepoChainDetail(chains []CrossRepoChain) string {
	longest := 0
	repos := map[string]bool{}
	for _, c := range chains {
		if len(c.Hops) > longest {
			longest = len(c.Hops)
		}
		for _, h := range c.Hops {
			repos[CrossRepoDisplayName(CrossRepoImpact{
				RelatedRepo:     h.RelatedRepo,
				RelatedRepoName: h.RelatedRepoName,
			})] = true
		}
	}
	if longest <= 1 {
		return "Reached one related repo"
	}
	if len(repos) == 1 {
		for repo := range repos {
			return fmt.Sprintf("Reached %s through %d hops", repo, longest)
		}
	}
	return fmt.Sprintf("Reached %d related repos through %d hops", len(repos), longest)
}

// crossRepoDetail intentionally says "Calls into" (outbound), not "Called
// from" - the symbol this signal belongs to is the caller.
func crossRepoDetail(impacts []CrossRepoImpact) string {
	if len(impacts) == 1 {
		return "Calls into " + CrossRepoDisplayName(impacts[0])
	}
	repos := map[string]bool{}
	for _, im := range impacts {
		repos[CrossRepoDisplayName(im)] = true
	}
	if len(repos) == 1 {
		for repo := range repos {
			return "Calls into " + repo
		}
	}
	return "Calls into related repositories"
}

// ChainSignature is a stable key for deduping chains at the hunk level.
func ChainSignature(c CrossRepoChain) string {
	out := ""
	for _, h := range c.Hops {
		out += h.RelatedRepo + "|" + h.CallSite + "|" + h.Handler + ";"
	}
	return out
}

// Renormalize recomputes MaxBlastRadiusRaw/BlastRadiusNorm (and the derived
// Combined) across every hunk after one or more hunks' raw blast radius
// changed post-scoring (see RecordCrossRepoImpact), then rebuilds the
// hunk-level cross-repo rollups from their symbols.
//
// This mirrors the normalization the scorer performs once over the whole diff:
// the highest BlastRadiusRaw across every hunk becomes every hunk's
// MaxBlastRadiusRaw. ReviewPriority's own Max/Norm are untouched - only blast
// radius can change here.
func Renormalize(report *Report) {
	if report == nil {
		return
	}
	var maxRaw float64
	var maxFile, maxHeader string
	var maxPriority float64
	var maxPriorityFile, maxPriorityHeader string
	for _, file := range report.Files {
		for _, hunk := range file.Hunks {
			if hunk.BlastRadiusRaw > maxRaw {
				maxRaw, maxFile, maxHeader = hunk.BlastRadiusRaw, file.Path, hunk.Header
			}
			if hunk.ReviewPriorityRaw > maxPriority {
				maxPriority, maxPriorityFile, maxPriorityHeader = hunk.ReviewPriorityRaw, file.Path, hunk.Header
			}
		}
	}
	for fi := range report.Files {
		file := &report.Files[fi]
		for hi := range file.Hunks {
			hunk := &file.Hunks[hi]
			hunk.MaxBlastRadiusRaw = maxRaw
			hunk.MaxBlastRadiusHunkFile = maxFile
			hunk.MaxBlastRadiusHunkHeader = maxHeader
			if maxRaw > 0 {
				hunk.BlastRadiusNorm = hunk.BlastRadiusRaw / maxRaw * 100
			} else {
				hunk.BlastRadiusNorm = 0
			}
			hunk.MaxReviewPriorityRaw = maxPriority
			hunk.MaxReviewPriorityHunkFile = maxPriorityFile
			hunk.MaxReviewPriorityHunkHeader = maxPriorityHeader
			if maxPriority > 0 {
				hunk.ReviewPriorityNorm = hunk.ReviewPriorityRaw / maxPriority * 100
			} else {
				hunk.ReviewPriorityNorm = 0
			}
			weights := hunk.Weights
			if weights.BlastRadius <= 0 && weights.ReviewPriority <= 0 {
				weights = DefaultWeights()
			}
			// HygieneMultiplier is a real [0,1] dampener: 0 legitimately means
			// "fully suppressed" (e.g. formatting-only), so it must never be
			// treated as an absent field here.
			hunk.Combined = (weights.BlastRadius*hunk.BlastRadiusNorm +
				weights.ReviewPriority*hunk.ReviewPriorityNorm) * hunk.HygieneMultiplier

			// Rebuilt from the symbols themselves rather than accumulated
			// incrementally, so this stays correct (no duplicates) no matter
			// how many times RecordCrossRepoImpact ran for this hunk.
			hunk.ImpactedRelatedRepos = nil
			hunk.ImpactedRepoChains = nil
			seenChains := map[string]bool{}
			for _, sym := range hunk.Symbols {
				hunk.ImpactedRelatedRepos = append(hunk.ImpactedRelatedRepos, sym.ImpactedRelatedRepos...)
				for _, chain := range sym.ImpactedRepoChains {
					sig := ChainSignature(chain)
					if seenChains[sig] {
						continue
					}
					seenChains[sig] = true
					hunk.ImpactedRepoChains = append(hunk.ImpactedRepoChains, chain)
				}
			}
		}
	}
}
