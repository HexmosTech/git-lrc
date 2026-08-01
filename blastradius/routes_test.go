package blastradius

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
	"github.com/HexmosTech/blastradius/score"
)

func TestRouteHandlersEmptyInput(t *testing.T) {
	got := routeHandlers(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestRouteHandlersParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"handler", "method", "path"},
		Rows: [][]string{
			{"pkg.AssignLicense", "POST", "/:id/assign"},
		},
	}}
	got := routeHandlers(context.Background(), q, []string{"pkg.AssignLicense"})
	info, ok := got["pkg.AssignLicense"]
	if !ok || info.Method != "POST" || info.Path != "/:id/assign" {
		t.Fatalf("unexpected result: %+v (ok=%v)", info, ok)
	}
	if info.String() != "POST /:id/assign" {
		t.Fatalf("String() = %q", info.String())
	}
}

func TestEntryPointFlagsEmptyInput(t *testing.T) {
	got := entryPointFlags(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestEntryPointFlagsParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"qn"},
		Rows:    [][]string{{"pkg.main"}},
	}}
	got := entryPointFlags(context.Background(), q, []string{"pkg.main", "pkg.Other"})
	if !got["pkg.main"] {
		t.Fatalf("expected pkg.main to be flagged as entry point")
	}
	if got["pkg.Other"] {
		t.Fatalf("expected pkg.Other to be absent/false")
	}
}

func TestEntryReachabilitySignalNoMatches(t *testing.T) {
	callers := []score.CallerContribution{{QualifiedName: "pkg.Regular", Depth: 1, Weight: 1.0}}
	got := entryReachabilitySignal(callers, map[string]RouteInfo{}, map[string]bool{})
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestEntryReachabilitySignalRouteAndEntryPoint(t *testing.T) {
	callers := []score.CallerContribution{
		{QualifiedName: "pkg.Handler", Depth: 1, Weight: 1.0},
		{QualifiedName: "pkg.main", Depth: 2, Weight: 0.5},
		{QualifiedName: "pkg.Irrelevant", Depth: 1, Weight: 1.0},
	}
	routes := map[string]RouteInfo{"pkg.Handler": {Method: "POST", Path: "/x"}}
	entryFlags := map[string]bool{"pkg.main": true}

	got := entryReachabilitySignal(callers, routes, entryFlags)
	if got == nil {
		t.Fatal("expected a non-nil signal")
	}
	if got.Name != "Reached from 2 service entry point(s)" {
		t.Fatalf("unexpected Name: %q", got.Name)
	}
	wantPoints := entryReachabilityUnit*1.0 + entryReachabilityUnit*0.5
	if got.Points != wantPoints {
		t.Fatalf("Points = %v, want %v", got.Points, wantPoints)
	}
}

func TestLastSegment(t *testing.T) {
	if got := lastSegment("home-shrsv-bin-LiveReview.cmd.main"); got != "main" {
		t.Fatalf("lastSegment = %q, want main", got)
	}
}
