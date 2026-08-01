package blastradius

import (
	"testing"

	"github.com/HexmosTech/blastradius/symbols"
)

func TestClassifyHunkHygieneNoMatch(t *testing.T) {
	hunk := Hunk{FilePath: "internal/api/users.go", Content: "-oldLogic()\n+newLogic()"}
	mult, sig := classifyHunkHygiene(hunk, nil)
	if mult != 1.0 || sig != nil {
		t.Fatalf("expected no hygiene match, got mult=%v sig=%+v", mult, sig)
	}
}

func TestClassifyHunkHygieneFormattingOnly(t *testing.T) {
	hunk := Hunk{
		FilePath: "internal/api/users.go",
		Content:  "-\tfoo(a,   b)\n+\tfoo(a, b)",
	}
	mult, sig := classifyHunkHygiene(hunk, nil)
	if sig == nil || sig.Name != "Formatting only" {
		t.Fatalf("expected Formatting only, got mult=%v sig=%+v", mult, sig)
	}
	if mult >= 0.5 {
		t.Fatalf("expected a strong dampener, got %v", mult)
	}
}

func TestClassifyHunkHygieneCommentsOnly(t *testing.T) {
	hunk := Hunk{
		FilePath: "internal/api/users.go",
		Content:  "+// explains the thing below\n+# also a comment style",
	}
	_, sig := classifyHunkHygiene(hunk, nil)
	if sig == nil || sig.Name != "Comments only" {
		t.Fatalf("expected Comments only, got %+v", sig)
	}
}

func TestClassifyHunkHygieneLoggingOnly(t *testing.T) {
	hunk := Hunk{
		FilePath: "internal/api/users.go",
		Content:  "+\tlog.Printf(\"created user %d\", id)\n+\tlogger.Info(\"done\")",
	}
	_, sig := classifyHunkHygiene(hunk, nil)
	if sig == nil || sig.Name != "Logging only" {
		t.Fatalf("expected Logging only, got %+v", sig)
	}
}

func TestClassifyHunkHygieneGeneratedPath(t *testing.T) {
	hunk := Hunk{FilePath: "internal/api/users.pb.go", Content: "+x := 1"}
	_, sig := classifyHunkHygiene(hunk, nil)
	if sig == nil || sig.Name != "Generated code" {
		t.Fatalf("expected Generated code, got %+v", sig)
	}
}

func TestClassifyHunkHygieneTestFile(t *testing.T) {
	hunk := Hunk{FilePath: "internal/api/users_test.go", Content: "+\tassert.Equal(t, 1, 1)"}
	_, sig := classifyHunkHygiene(hunk, nil)
	if sig == nil || sig.Name != "Test-only file" {
		t.Fatalf("expected Test-only file, got %+v", sig)
	}
}

func TestClassifyHunkHygieneDeadCodeRemoval(t *testing.T) {
	hunk := Hunk{FilePath: "internal/api/users.go", NewLines: 0, Content: "-func unused() {}"}
	touched := []symbols.Symbol{{QualifiedName: "pkg.unused", InDegree: 0, IsEntryPoint: false, IsTest: false}}
	_, sig := classifyHunkHygiene(hunk, touched)
	if sig == nil || sig.Name != "Dead code removal" {
		t.Fatalf("expected Dead code removal, got %+v", sig)
	}
}

func TestClassifyHunkHygieneDeadCodeRemovalSkippedForLiveSymbol(t *testing.T) {
	hunk := Hunk{FilePath: "internal/api/users.go", NewLines: 0, Content: "-func used() {}"}
	touched := []symbols.Symbol{{QualifiedName: "pkg.used", InDegree: 3}}
	_, sig := classifyHunkHygiene(hunk, touched)
	if sig != nil {
		t.Fatalf("expected no dead-code signal for a symbol with callers, got %+v", sig)
	}
}

func TestIsFormattingOnlyRequiresSameLineCount(t *testing.T) {
	if isFormattingOnly([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("expected false for mismatched line counts")
	}
	if isFormattingOnly(nil, nil) {
		t.Fatal("expected false for empty input")
	}
}
