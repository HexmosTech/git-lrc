package blastradius

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/HexmosTech/blastradius/client"
)

// architectureContext is a lookup structure built once per Report from a
// single `get_architecture` call - this is architecture-wide data, never
// per-hunk or per-symbol, so it's fetched exactly once and reused.
type architectureContext struct {
	hotspotFanIn map[string]int                      // qualified_name -> fan_in
	layerByName  map[string]client.ArchitectureLayer // package name -> layer info
	entryPointQN map[string]bool                     // qualified_name -> true
}

// fetchArchitectureContext calls get_architecture once and indexes the
// result. Best-effort: on any error it returns an empty (harmless) context
// rather than failing the whole report - architecture context is enrichment,
// not a hard requirement.
func fetchArchitectureContext(ctx context.Context, c *client.Client) *architectureContext {
	empty := &architectureContext{
		hotspotFanIn: map[string]int{},
		layerByName:  map[string]client.ArchitectureLayer{},
		entryPointQN: map[string]bool{},
	}
	arch, err := c.GetArchitecture(ctx, []string{"hotspots", "layers", "entry_points"})
	if err != nil {
		return empty
	}
	for _, h := range arch.Hotspots {
		if h.QualifiedName != "" {
			empty.hotspotFanIn[h.QualifiedName] = h.FanIn
		}
	}
	for _, l := range arch.Layers {
		if l.Name != "" {
			empty.layerByName[l.Name] = l
		}
	}
	for _, ep := range arch.EntryPoints {
		if ep.QualifiedName != "" {
			empty.entryPointQN[ep.QualifiedName] = true
		}
	}
	return empty
}

// hotspotSignal fires when qn is itself one of the repo's precomputed
// top-fan-in hotspots - a "this is a known hub in the whole codebase" signal
// independent of (and complementary to) the diff-scoped depth-3 fan-in
// already computed, since a symbol can look unremarkable within a single
// diff while being one of the most-called symbols repo-wide.
func (a *architectureContext) hotspotSignal(qn string) *Signal {
	fanIn, ok := a.hotspotFanIn[qn]
	if !ok {
		return nil
	}
	return &Signal{
		Name:     "Repo-wide hotspot",
		Detail:   fmt.Sprintf("fan-in=%d across the whole codebase, independent of this diff", fanIn),
		Points:   math.Log1p(float64(fanIn)) * 0.5,
		Category: "architecture",
	}
}

// layerSignal fires when qn's package resolves to the "core" or "api"
// architectural layer - a free, cross-language importance classification
// computed by the tool, for exactly the "matters regardless of caller
// count" case. get_architecture's layer names are coarse, single-segment
// package identifiers (e.g. "provider_input", not "internal/provider_input"),
// so this matches against any segment of the symbol's package path rather
// than requiring an exact full-path match.
func (a *architectureContext) layerSignal(qn string) *Signal {
	pkg := packageOf(qn)
	if pkg == "" {
		return nil
	}
	for segment := range strings.SplitSeq(pkg, "/") {
		layer, ok := a.layerByName[segment]
		if !ok {
			continue
		}
		if layer.Layer != "core" && layer.Layer != "api" {
			continue
		}
		return &Signal{
			Name:     "Architectural layer",
			Detail:   fmt.Sprintf("package %q is in the %q layer (%s)", segment, layer.Layer, layer.Reason),
			Points:   1.5,
			Category: "architecture",
		}
	}
	return nil
}
