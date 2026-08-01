package score

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

// fakeQuerier simulates depth-exact CALLS queries: depth 1 finds "direct",
// depth 2 finds "indirect", depth 3 finds nothing new.
type fakeQuerier struct{ calls int }

func (f *fakeQuerier) QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error) {
	f.calls++
	switch {
	case strings.Contains(cypher, "*1..1"):
		return &client.QueryResult{
			Columns: []string{"symbol", "caller"},
			Rows: [][]string{
				{"pkg.Target", "pkg.DirectCallerA"},
				{"pkg.Target", "pkg.DirectCallerB"},
			},
		}, nil
	case strings.Contains(cypher, "*2..2"):
		return &client.QueryResult{
			Columns: []string{"symbol", "caller"},
			Rows: [][]string{
				{"pkg.Target", "pkg.IndirectCallerC"},
				// same caller found again at depth 2 - should not double count
				// and should keep the smaller depth (1) already recorded.
				{"pkg.Target", "pkg.DirectCallerA"},
			},
		}, nil
	default:
		return &client.QueryResult{Columns: []string{"symbol", "caller"}}, nil
	}
}

func TestFanInDecayAndDedup(t *testing.T) {
	fq := &fakeQuerier{}
	cfg := Config{MaxDepth: 3, Decay: 0.5}

	got, err := FanIn(context.Background(), fq, []string{"pkg.Target"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if fq.calls != 3 {
		t.Fatalf("expected exactly 3 batched queries (one per depth), got %d", fq.calls)
	}

	s := got["pkg.Target"]
	if s == nil {
		t.Fatal("missing score for pkg.Target")
	}
	// 2 direct callers (weight 1.0 each) + 1 indirect caller (weight 0.5).
	wantLinear := 2*1.0 + 1*0.5
	if math.Abs(s.LinearSum-wantLinear) > 1e-9 {
		t.Fatalf("LinearSum = %v, want %v", s.LinearSum, wantLinear)
	}
	// cfg.Transform was unset, so normalized() falls back to math.Sqrt.
	wantRaw := math.Sqrt(wantLinear)
	if math.Abs(s.Raw-wantRaw) > 1e-9 {
		t.Fatalf("Raw = %v, want sqrt(LinearSum) = %v", s.Raw, wantRaw)
	}
	if len(s.Callers) != 3 {
		t.Fatalf("expected 3 distinct callers (dedup across depths), got %d: %+v", len(s.Callers), s.Callers)
	}
	for _, c := range s.Callers {
		if c.QualifiedName == "pkg.DirectCallerA" && c.Depth != 1 {
			t.Fatalf("DirectCallerA should keep depth 1 (its shallowest), got %d", c.Depth)
		}
	}
}

// hubQuerier simulates a "hairball" symbol with many direct callers alongside
// a background symbol with few, to verify the default sqrt transform
// compresses the gap between them without inverting their order.
type hubQuerier struct{ hubCallers, backgroundCallers int }

func (q *hubQuerier) QueryGraph(ctx context.Context, cypher string, maxRows int) (*client.QueryResult, error) {
	if !strings.Contains(cypher, "*1..1") {
		return &client.QueryResult{Columns: []string{"symbol", "caller"}}, nil
	}
	var rows [][]string
	for i := 0; i < q.hubCallers; i++ {
		rows = append(rows, []string{"pkg.Hub", fmt.Sprintf("pkg.HubCaller%d", i)})
	}
	for i := 0; i < q.backgroundCallers; i++ {
		rows = append(rows, []string{"pkg.Background", fmt.Sprintf("pkg.BgCaller%d", i)})
	}
	return &client.QueryResult{Columns: []string{"symbol", "caller"}, Rows: rows}, nil
}

func TestFanInTransformCompressesHubDominance(t *testing.T) {
	q := &hubQuerier{hubCallers: 50, backgroundCallers: 2}
	got, err := FanIn(context.Background(), q, []string{"pkg.Hub", "pkg.Background"}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	hub, bg := got["pkg.Hub"], got["pkg.Background"]

	if hub.LinearSum != 50 || bg.LinearSum != 2 {
		t.Fatalf("LinearSum = %v/%v, want 50/2", hub.LinearSum, bg.LinearSum)
	}
	// Linear ratio is 25x; sqrt should compress that substantially while
	// preserving order (hub still scores higher).
	linearRatio := hub.LinearSum / bg.LinearSum
	rawRatio := hub.Raw / bg.Raw
	if rawRatio >= linearRatio {
		t.Fatalf("transform did not compress the gap: rawRatio=%v linearRatio=%v", rawRatio, linearRatio)
	}
	if hub.Raw <= bg.Raw {
		t.Fatalf("transform must preserve ordering: hub.Raw=%v should exceed bg.Raw=%v", hub.Raw, bg.Raw)
	}
	if math.Abs(hub.Raw-math.Sqrt(50)) > 1e-9 {
		t.Fatalf("hub.Raw = %v, want sqrt(50) = %v", hub.Raw, math.Sqrt(50))
	}
}

func TestFanInEmptyInput(t *testing.T) {
	fq := &fakeQuerier{}
	got, err := FanIn(context.Background(), fq, nil, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
	if fq.calls != 0 {
		t.Fatalf("expected no queries for empty input, got %d", fq.calls)
	}
}
