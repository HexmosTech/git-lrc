package story

import (
	"strings"
	"testing"
)

type testProvider struct {
	id string
}

func (p testProvider) ID() string {
	return p.id
}

func (p testProvider) Discover(DiscoverOptions) ([]Source, error) {
	return nil, nil
}

func (p testProvider) ListSessions(ListSessionsOptions) ([]SessionSummary, error) {
	return nil, nil
}

func (p testProvider) InspectSession(InspectSessionOptions) (*SessionInspect, error) {
	return nil, nil
}

func (p testProvider) ExportSession(ExportSessionOptions) (*CommonChat, error) {
	return nil, nil
}

func TestNewRegistryRejectsDuplicateProviderIDs(t *testing.T) {
	_, err := NewRegistry(testProvider{id: "dup"}, testProvider{id: "dup"})
	if err == nil {
		t.Fatal("expected duplicate provider registration to fail")
	}
	if !strings.Contains(err.Error(), "duplicate story provider id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRegistryAllowsUniqueProviderIDs(t *testing.T) {
	registry, err := NewRegistry(testProvider{id: "one"}, testProvider{id: "two"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := registry.ProviderIDs()
	if len(ids) != 2 {
		t.Fatalf("expected two provider ids, got %d", len(ids))
	}
	if ids[0] != "one" || ids[1] != "two" {
		t.Fatalf("unexpected provider ids: %v", ids)
	}
}
