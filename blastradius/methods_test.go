package blastradius

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

type fakeMethodsQuerier struct {
	result *client.QueryResult
}

func (f *fakeMethodsQuerier) QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error) {
	return f.result, nil
}

func TestMethodsByParentClassEmptyInput(t *testing.T) {
	got := methodsByParentClass(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map for empty input, got %v", got)
	}
}

func TestFileCouplingBonusEmptyInput(t *testing.T) {
	got := fileCouplingBonus(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map for empty input, got %v", got)
	}
}

func TestFileCouplingBonusSumsAndSaturates(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"file", "coupling_score"},
		Rows: [][]string{
			{"a.go", "0.5"},
			{"a.go", "0.5"},
			{"b.go", "1.0"},
		},
	}}
	got := fileCouplingBonus(context.Background(), q, []string{"a.go", "b.go"})
	// a.go: sqrt(0.5+0.5) = 1.0; b.go: sqrt(1.0) = 1.0 - equal despite b.go
	// having one stronger single coupling vs a.go's two weaker ones.
	if got["a.go"] != 1.0 || got["b.go"] != 1.0 {
		t.Fatalf("unexpected bonuses: %v", got)
	}
}

func TestMethodsByParentClassGroupsByParent(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"parent", "qn"},
		Rows: [][]string{
			{"pkg.Foo", "pkg.Foo.Bar"},
			{"pkg.Foo", "pkg.Foo.Baz"},
			{"pkg.Other", "pkg.Other.Qux"},
		},
	}}

	got := methodsByParentClass(context.Background(), q, []string{"pkg.Foo", "pkg.Other"})
	if len(got["pkg.Foo"]) != 2 {
		t.Fatalf("expected 2 methods for pkg.Foo, got %v", got["pkg.Foo"])
	}
	if len(got["pkg.Other"]) != 1 {
		t.Fatalf("expected 1 method for pkg.Other, got %v", got["pkg.Other"])
	}
}
