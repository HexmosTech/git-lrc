package reviewquery

import "testing"

func TestParseTrailingFlags(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantPos []string
		from    string
		to      string
		rng     string
		json    bool
	}{
		{"none", []string{"stats"}, []string{"stats"}, "", "", "", false},
		{"trailing json", []string{"stats", "--json"}, []string{"stats"}, "", "", "", true},
		{"from value", []string{"stats", "--from", "2024-01-01"}, []string{"stats"}, "2024-01-01", "", "", false},
		{"from equals", []string{"stats", "--from=2024-01-01"}, []string{"stats"}, "2024-01-01", "", "", false},
		{"range+json", []string{"q", "--range", "main...dev", "--json"}, []string{"q"}, "", "", "main...dev", true},
		{"to", []string{"stats", "--to=2025-12-31"}, []string{"stats"}, "", "2025-12-31", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonOut := false
			f := Filter{}
			pos := parseTrailingFlags(tc.args, &jsonOut, &f)
			if len(pos) != len(tc.wantPos) || (len(pos) > 0 && pos[0] != tc.wantPos[0]) {
				t.Errorf("positionals = %v; want %v", pos, tc.wantPos)
			}
			if f.From != tc.from || f.To != tc.to || f.Range != tc.rng || jsonOut != tc.json {
				t.Errorf("got from=%q to=%q range=%q json=%v; want from=%q to=%q range=%q json=%v",
					f.From, f.To, f.Range, jsonOut, tc.from, tc.to, tc.rng, tc.json)
			}
		})
	}
}
