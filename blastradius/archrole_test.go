package blastradius

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

func signalNames(signals []Signal) map[string]bool {
	names := make(map[string]bool, len(signals))
	for _, s := range signals {
		names[s.Name] = true
	}
	return names
}

func TestArchRoleSignalsAuth(t *testing.T) {
	names := signalNames(archRoleSignals("internal/api/auth/session.go"))
	if !names["Authentication-related"] {
		t.Fatalf("expected Authentication-related signal, got %v", names)
	}
}

func TestArchRoleSignalsPersistence(t *testing.T) {
	names := signalNames(archRoleSignals("storage/users/user_store.go"))
	if !names["Persistence layer"] {
		t.Fatalf("expected Persistence layer signal, got %v", names)
	}
}

func TestArchRoleSignalsBuild(t *testing.T) {
	names := signalNames(archRoleSignals("Makefile"))
	if !names["Build system"] {
		t.Fatalf("expected Build system signal, got %v", names)
	}
	names = signalNames(archRoleSignals(".github/workflows/ci.yml"))
	if !names["Build system"] {
		t.Fatalf("expected Build system signal for CI workflow, got %v", names)
	}
}

func TestArchRoleSignalsSchema(t *testing.T) {
	names := signalNames(archRoleSignals("migrations/0001_init.sql"))
	if !names["Schema change"] {
		t.Fatalf("expected Schema change signal, got %v", names)
	}
}

func TestArchRoleSignalsNoMatch(t *testing.T) {
	signals := archRoleSignals("internal/util/stringutil.go")
	if len(signals) != 0 {
		t.Fatalf("expected no signals for an unremarkable path, got %+v", signals)
	}
}

func TestArchRoleSignalsAvoidsFalsePositiveOnSubstring(t *testing.T) {
	// "expression" contains "session" as a raw substring but shouldn't match
	// the boundary-anchored auth pattern.
	names := signalNames(archRoleSignals("internal/eval/expression_parser.go"))
	if names["Authentication-related"] {
		t.Fatalf("expected no false-positive auth match for expression_parser.go, got %v", names)
	}
}

func TestSymbolsThatWriteDataEmptyInput(t *testing.T) {
	got := symbolsThatWriteData(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestSymbolsThatWriteDataParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"qn"},
		Rows:    [][]string{{"pkg.Save"}},
	}}
	got := symbolsThatWriteData(context.Background(), q, []string{"pkg.Save", "pkg.Read"})
	if !got["pkg.Save"] || got["pkg.Read"] {
		t.Fatalf("unexpected result: %v", got)
	}
}
