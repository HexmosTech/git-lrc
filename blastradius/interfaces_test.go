package blastradius

import (
	"context"
	"testing"

	"github.com/HexmosTech/blastradius/client"
)

func TestImplementedInterfacesEmptyInput(t *testing.T) {
	got := implementedInterfaces(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestImplementedInterfacesParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"impl", "iface", "iface_name"},
		Rows: [][]string{
			{"pkg.GiteaClient", "pkg.OutputClient", "OutputClient"},
		},
	}}
	got := implementedInterfaces(context.Background(), q, []string{"pkg.GiteaClient"})
	infos, ok := got["pkg.GiteaClient"]
	if !ok || len(infos) != 1 || infos[0].interfaceQN != "pkg.OutputClient" || infos[0].interfaceName != "OutputClient" {
		t.Fatalf("unexpected result: %+v (ok=%v)", infos, ok)
	}
}

func TestImplementerCountsEmptyInput(t *testing.T) {
	got := implementerCounts(context.Background(), nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestImplementerCountsParsesRows(t *testing.T) {
	q := &fakeMethodsQuerier{result: &client.QueryResult{
		Columns: []string{"iface", "implementer_count"},
		Rows:    [][]string{{"pkg.OutputClient", "9"}},
	}}
	got := implementerCounts(context.Background(), q, []string{"pkg.OutputClient"})
	if got["pkg.OutputClient"] != 9 {
		t.Fatalf("unexpected count: %v", got)
	}
}
