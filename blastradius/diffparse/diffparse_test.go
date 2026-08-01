package diffparse

import (
	"os"
	"testing"
)

const sampleDiff = `diff --git a/foo.go b/foo.go
index 111..222 100644
--- a/foo.go
+++ b/foo.go
@@ -10,3 +10,4 @@ func Foo() {
 	a := 1
-	b := 2
+	b := 3
+	c := 4
 }
diff --git a/bar.go b/bar.go
new file mode 100644
index 000..333
--- /dev/null
+++ b/bar.go
@@ -0,0 +1,2 @@
+package bar
+
`

func TestParseBasic(t *testing.T) {
	files, err := Parse([]byte(sampleDiff))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	foo := files[0]
	if foo.Path != "foo.go" || foo.OldPath != "foo.go" {
		t.Fatalf("unexpected foo paths: %+v", foo)
	}
	if len(foo.Hunks) != 1 {
		t.Fatalf("expected 1 hunk in foo.go, got %d", len(foo.Hunks))
	}
	h := foo.Hunks[0]
	if h.OldStart != 10 || h.OldLines != 3 || h.NewStart != 10 || h.NewLines != 4 {
		t.Fatalf("unexpected hunk range: %+v", h)
	}

	bar := files[1]
	if !bar.IsNew {
		t.Fatalf("expected bar.go to be marked IsNew")
	}
	if bar.Path != "bar.go" {
		t.Fatalf("unexpected bar path: %q", bar.Path)
	}
	if len(bar.Hunks) != 1 || bar.Hunks[0].NewStart != 1 || bar.Hunks[0].NewLines != 2 {
		t.Fatalf("unexpected bar hunk: %+v", bar.Hunks)
	}
}

func TestParseRealFixtures(t *testing.T) {
	fixtures := []string{
		"../internal/testfixtures/sample-core.diff",
		"../internal/testfixtures/sample-leaf.diff",
		"../internal/testfixtures/sample-mixed.diff",
	}
	for _, path := range fixtures {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		files, err := Parse(data)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		if len(files) == 0 {
			t.Fatalf("%s: expected at least one file", path)
		}
		totalHunks := 0
		for _, f := range files {
			totalHunks += len(f.Hunks)
			for _, h := range f.Hunks {
				if h.NewLines > 0 && h.NewStart == 0 {
					t.Errorf("%s: file %s hunk %q has NewStart=0 with NewLines>0", path, f.Path, h.Header)
				}
			}
		}
		if totalHunks == 0 {
			t.Fatalf("%s: expected at least one hunk across %d files", path, len(files))
		}
		t.Logf("%s: %d files, %d hunks", path, len(files), totalHunks)
	}
}
