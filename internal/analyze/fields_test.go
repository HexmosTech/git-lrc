package analyze

import (
	"testing"
)

func TestIsStateLikeName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"status", true},
		{"state", true},
		{"stage", true},
		{"phase", true},
		{"type", true},
		{"kind", true},
		{"role", true},
		{"category", true},
		{"mode", true},
		{"level", true},
		{"priority", true},
		{"severity", true},
		{"action", true},
		{"event", true},
		{"result", true},
		{"outcome", true},
		{"disposition", true},
		{"resolution", true},
		{"review_status", true},
		{"order_state", true},
		{"payment_type", true},
		{"ticket_kind", true},
		{"build_stage", true},
		{"user_role", true},
		{"item_category", true},
		{"run_mode", true},
		// Not state-like
		{"name", false},
		{"email", false},
		{"created_at", false},
		{"description", false},
		{"amount", false},
		{"url", false},
		{"id", false},
		{"org_id", false},
		{"metadata", false},
		{"body", false},
		{"title", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStateLikeName(tt.name)
			if got != tt.want {
				t.Errorf("isStateLikeName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsTextHeavy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"short", "hello", false},
		{"exact_100", string(make([]byte, 100)), false},
		{"over_100", string(make([]byte, 101)), true},
		{"4_spaces", "a b c d", false},
		{"5_spaces", "a b c d e", true},
		{"tabs_count", "a\tb\tc\td\te", true},
		{"mixed_whitespace", "a b\tc d\te", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTextHeavy(tt.input)
			if got != tt.want {
				t.Errorf("isTextHeavy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePGArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"empty_braces", "{}", nil},
		{"single", "{hello}", []string{"hello"}},
		{"multiple", "{a,b,c}", []string{"a", "b", "c"}},
		{"quoted", `{"hello","world"}`, []string{"hello", "world"}},
		{"mixed", `{plain,"quoted",another}`, []string{"plain", "quoted", "another"}},
		{"with_spaces", `{a b,c d}`, []string{"a b", "c d"}},
		{"empty_element", `{a,,b}`, []string{"a", "", "b"}},
		{"no_braces", `a,b,c`, []string{"a", "b", "c"}},
		{"single_no_braces", `hello`, []string{"hello"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePGArray(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return // both nil/empty
			}
			if len(got) != len(tt.want) {
				t.Errorf("parsePGArray(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parsePGArray(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseFloats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected length; exact values checked separately
	}{
		{"empty", "", 0},
		{"empty_braces", "{}", 0},
		{"single", "{0.5}", 1},
		{"multiple", "{0.1,0.2,0.3}", 3},
		{"integer_values", "{1,2,3}", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFloats(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseFloats(%q) returned %d values, want %d", tt.input, len(got), tt.want)
			}
		})
	}

	// Check specific values
	t.Run("values", func(t *testing.T) {
		got := parseFloats("{0.1,0.2,0.3}")
		expected := []float64{0.1, 0.2, 0.3}
		for i, v := range got {
			if v < expected[i]-0.001 || v > expected[i]+0.001 {
				t.Errorf("parseFloats[%d] = %f, want ~%f", i, v, expected[i])
			}
		}
	})
}
