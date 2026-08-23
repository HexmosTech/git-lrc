package analyze

import (
	"testing"
)

func TestInferType(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"nil", nil, "null"},
		{"bool_true", true, "boolean"},
		{"bool_false", false, "boolean"},
		{"number_int", float64(42), "number"},
		{"number_float", float64(3.14), "number"},
		{"string", "hello", "string"},
		{"empty_string", "", "string"},
		{"array", []interface{}{}, "array"},
		{"object", map[string]interface{}{}, "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferType(tt.input)
			if got != tt.want {
				t.Errorf("inferType(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWalkJSON_Simple(t *testing.T) {
	input := map[string]interface{}{
		"name": "test",
		"age":  float64(30),
	}

	paths := make(map[string]map[string]int)
	walkJSON(input, "$", paths)

	if _, ok := paths["$.name"]; !ok {
		t.Error("missing path $.name")
	} else if paths["$.name"]["string"] != 1 {
		t.Errorf("$.name type = %v, want string:1", paths["$.name"])
	}

	if _, ok := paths["$.age"]; !ok {
		t.Error("missing path $.age")
	} else if paths["$.age"]["number"] != 1 {
		t.Errorf("$.age type = %v, want number:1", paths["$.age"])
	}
}

func TestWalkJSON_Nested(t *testing.T) {
	input := map[string]interface{}{
		"user": map[string]interface{}{
			"profile": map[string]interface{}{
				"email": "test@example.com",
				"score": float64(95),
			},
		},
	}

	paths := make(map[string]map[string]int)
	walkJSON(input, "$", paths)

	expectedPaths := []string{"$.user", "$.user.profile", "$.user.profile.email", "$.user.profile.score"}
	for _, p := range expectedPaths {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %s", p)
		}
	}

	if paths["$.user"]["object"] != 1 {
		t.Errorf("$.user type = %v, want object:1", paths["$.user"])
	}
	if paths["$.user.profile.email"]["string"] != 1 {
		t.Errorf("$.user.profile.email type = %v, want string:1", paths["$.user.profile.email"])
	}
}

func TestWalkJSON_Array(t *testing.T) {
	input := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": float64(1)},
			map[string]interface{}{"id": float64(2)},
		},
	}

	paths := make(map[string]map[string]int)
	walkJSON(input, "$", paths)

	// walkJSON does not register the array container path itself,
	// only the paths inside each element.
	if _, ok := paths["$.items[].id"]; !ok {
		t.Error("missing path $.items[].id")
	} else if paths["$.items[].id"]["number"] != 2 {
		t.Errorf("$.items[].id count = %d, want 2", paths["$.items[].id"]["number"])
	}
}

func TestCollectLeafValues(t *testing.T) {
	input := map[string]interface{}{
		"status": "active",
		"meta": map[string]interface{}{
			"count": float64(5),
		},
	}

	pathValues := make(map[string]map[string]int)
	collectLeafValues(input, "$", pathValues)

	if vals, ok := pathValues["$.status"]; !ok {
		t.Error("missing $.status")
	} else if vals["active"] != 1 {
		t.Errorf("$.status[active] = %d, want 1", vals["active"])
	}

	if vals, ok := pathValues["$.meta.count"]; !ok {
		t.Error("missing $.meta.count")
	} else if vals["5"] != 1 {
		t.Errorf("$.meta.count[5] = %d, want 1", vals["5"])
	}
}

func TestCollectLeafValues_Array(t *testing.T) {
	// collectLeafValues handles arrays by recursing with "[]" prefix,
	// but only records leaf values that are map values (not direct array elements).
	input := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"name": "first"},
			map[string]interface{}{"name": "second"},
		},
	}

	pathValues := make(map[string]map[string]int)
	collectLeafValues(input, "$", pathValues)

	if vals, ok := pathValues["$.items[].name"]; !ok {
		t.Error("missing $.items[].name")
	} else {
		if vals["first"] != 1 {
			t.Errorf("$.items[].name[first] = %d, want 1", vals["first"])
		}
		if vals["second"] != 1 {
			t.Errorf("$.items[].name[second] = %d, want 1", vals["second"])
		}
	}
}

func TestSampleQuery_SmallTable(t *testing.T) {
	q := sampleQuery("col", "tbl", "WHERE col IS NOT NULL", 100, 50)
	if !contains(q, "LIMIT 50") {
		t.Errorf("expected LIMIT 50 in query: %s", q)
	}
	if contains(q, "TABLESAMPLE") {
		t.Errorf("unexpected TABLESAMPLE for small table: %s", q)
	}
}

func TestSampleQuery_LargeTable(t *testing.T) {
	q := sampleQuery("col", "tbl", "WHERE col IS NOT NULL", 100000, 50)
	if !contains(q, "TABLESAMPLE BERNOULLI") {
		t.Errorf("expected TABLESAMPLE for large table: %s", q)
	}
	if !contains(q, "LIMIT 50") {
		t.Errorf("expected LIMIT 50 in query: %s", q)
	}
}

func TestSampleQuery_SizeFilter(t *testing.T) {
	q := sampleQuery("col", "tbl", "", 100, 50)
	if !contains(q, "octet_length") {
		t.Errorf("expected octet_length filter: %s", q)
	}
}

func TestSampleQuery_AppendsToWhere(t *testing.T) {
	q := sampleQuery("col", "tbl", "WHERE col IS NOT NULL", 100, 50)
	if !contains(q, "col IS NOT NULL") {
		t.Errorf("expected original WHERE clause preserved: %s", q)
	}
	if !contains(q, "octet_length") {
		t.Errorf("expected octet_length appended: %s", q)
	}
}

// Guards against regressing to the old 10KB cutoff that excluded normal rows.
func TestSampleQuery_SizeFilterIsOneMebibyte(t *testing.T) {
	q := sampleQuery("col", "tbl", "", 100, 50)
	if !contains(q, "octet_length(col::text) < 1048576") {
		t.Errorf("expected 1 MiB (1048576) size filter, got: %s", q)
	}
	if contains(q, "10240") {
		t.Errorf("old 10KB size filter still present: %s", q)
	}
}

// Guards against losing ORDER BY random(), which is what makes LIMIT unbiased.
func TestSampleQuery_OrderByRandom_SmallTable(t *testing.T) {
	q := sampleQuery("col", "tbl", "WHERE col IS NOT NULL", 100, 50)
	if !contains(q, "ORDER BY random()") {
		t.Errorf("expected ORDER BY random() in small-table query: %s", q)
	}
	if !contains(q, "ORDER BY random() LIMIT 50") {
		t.Errorf("expected ORDER BY random() to precede LIMIT: %s", q)
	}
}

// Guards the same thing on the TABLESAMPLE branch - thinning alone isn't unbiased.
func TestSampleQuery_OrderByRandom_LargeTable(t *testing.T) {
	q := sampleQuery("col", "tbl", "WHERE col IS NOT NULL", 100000, 50)
	if !contains(q, "TABLESAMPLE BERNOULLI") {
		t.Errorf("expected TABLESAMPLE for large table: %s", q)
	}
	if !contains(q, "ORDER BY random() LIMIT 50") {
		t.Errorf("expected ORDER BY random() to precede LIMIT: %s", q)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
