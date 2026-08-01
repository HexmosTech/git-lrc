package blastradius

import (
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

func TestHotspotSignal(t *testing.T) {
	a := &architectureContext{hotspotFanIn: map[string]int{"pkg.Hub": 230}}
	if a.hotspotSignal("pkg.Unknown") != nil {
		t.Fatal("expected nil for a symbol not in the hotspot list")
	}
	sig := a.hotspotSignal("pkg.Hub")
	if sig == nil || sig.Category != "architecture" || sig.Points <= 0 {
		t.Fatalf("unexpected signal: %+v", sig)
	}
}

func TestLayerSignalMatchesAnySegment(t *testing.T) {
	a := &architectureContext{layerByName: map[string]client.ArchitectureLayer{
		"provider_input": {Name: "provider_input", Layer: "core", Reason: "high fan-in"},
		"internal":       {Name: "internal", Layer: "internal", Reason: "fan-in=150, fan-out=180"},
	}}
	sig := a.layerSignal("home-shrsv-bin-LiveReview.internal.provider_input.gitea.GiteaOutputClient.Post")
	if sig == nil {
		t.Fatal("expected a layer signal for a package matching the 'provider_input' segment")
	}
	if sig.Points != 1.5 {
		t.Fatalf("unexpected Points: %v", sig.Points)
	}
}

func TestLayerSignalSkipsInternalAndEntryLayers(t *testing.T) {
	a := &architectureContext{layerByName: map[string]client.ArchitectureLayer{
		"jobqueue": {Name: "jobqueue", Layer: "entry", Reason: "only outbound calls"},
	}}
	if sig := a.layerSignal("pkg.jobqueue.Worker.Run"); sig != nil {
		t.Fatalf("expected nil for an 'entry' layer (not core/api), got %+v", sig)
	}
}

func TestLayerSignalNoMatch(t *testing.T) {
	a := &architectureContext{layerByName: map[string]client.ArchitectureLayer{}}
	if sig := a.layerSignal("pkg.unknown.Foo"); sig != nil {
		t.Fatalf("expected nil, got %+v", sig)
	}
}
