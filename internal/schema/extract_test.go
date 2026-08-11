package schema

import "testing"

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "users", `"users"`},
		{"with_spaces", "my table", `"my table"`},
		{"reserved_word", "select", `"select"`},
		{"empty", "", `""`},
		{"with_dot", "public.users", `"public.users"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteIdent(tt.input)
			if got != tt.want {
				t.Errorf("QuoteIdent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
