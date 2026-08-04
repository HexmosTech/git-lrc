package blastradius

import (
	"testing"

	"github.com/HexmosTech/blastradius/symbols"
)

func TestDetectDeclRenameCleanMatch(t *testing.T) {
	hunk := Hunk{
		NewStart: 10,
		Content:  " import \"fmt\"\n-func RunHelp() {\n+func RuHelp() {\n \tfmt.Println()\n }",
	}
	sym := symbols.Symbol{Name: "RuHelp", StartLine: 11, EndLine: 14}

	rn := detectDeclRename(hunk, sym)
	if rn == nil {
		t.Fatal("expected a detected rename, got nil")
	}
	if rn.OldName != "RunHelp" || rn.NewName != "RuHelp" {
		t.Fatalf("got OldName=%q NewName=%q, want OldName=RunHelp NewName=RuHelp", rn.OldName, rn.NewName)
	}
}

func TestDetectDeclRenameStructMatch(t *testing.T) {
	hunk := Hunk{
		NewStart: 22,
		Content:  "-type category struct {\n+type baldcategory struct {\n \tlabel string\n }",
	}
	sym := symbols.Symbol{Name: "baldcategory", StartLine: 22, EndLine: 25}

	rn := detectDeclRename(hunk, sym)
	if rn == nil {
		t.Fatal("expected a detected rename, got nil")
	}
	if rn.OldName != "category" || rn.NewName != "baldcategory" {
		t.Fatalf("got OldName=%q NewName=%q, want OldName=category NewName=baldcategory", rn.OldName, rn.NewName)
	}
}

func TestDetectDeclRenameIgnoresUnrelatedRenameElsewhereInHunk(t *testing.T) {
	// The symbol's own declaration line (20) is untouched context; only an
	// unrelated local variable a few lines down was renamed. The symbol
	// itself was not renamed, so no DeclRename should fire for it.
	hunk := Hunk{
		NewStart: 20,
		Content:  " func RunHelp() {\n-\tx := oldVar\n+\ty := newVar\n }",
	}
	sym := symbols.Symbol{Name: "RunHelp", StartLine: 20, EndLine: 23}

	if rn := detectDeclRename(hunk, sym); rn != nil {
		t.Fatalf("expected no rename detected, got %+v", rn)
	}
}

func TestDetectDeclRenameSkipsMultiLineSignatureReshuffle(t *testing.T) {
	// Both the removed and added blocks span 2 lines (an equal-count replace
	// block), but the declaration line itself gained a parameter - not a
	// clean single-identifier swap - so this must not be mistaken for a
	// rename.
	hunk := Hunk{
		NewStart: 30,
		Content:  "-func something(a, b int,\n-\tc string) {\n+func something(a, b, c int,\n+\td string) {\n }",
	}
	sym := symbols.Symbol{Name: "something", StartLine: 30, EndLine: 34}

	if rn := detectDeclRename(hunk, sym); rn != nil {
		t.Fatalf("expected no rename detected for a multi-line signature reshuffle, got %+v", rn)
	}
}

func TestDetectDeclRenameSkipsFormattingOnlyChange(t *testing.T) {
	// Same identifiers, only whitespace differs - not a rename.
	hunk := Hunk{
		NewStart: 5,
		Content:  "-func RunHelp( ) {\n+func RunHelp() {",
	}
	sym := symbols.Symbol{Name: "RunHelp", StartLine: 5, EndLine: 8}

	if rn := detectDeclRename(hunk, sym); rn != nil {
		t.Fatalf("expected no rename detected for a formatting-only change, got %+v", rn)
	}
}

func TestDetectDeclRenameSkipsMultiTokenChange(t *testing.T) {
	// Two identifiers differ on the declaration line - not a clean single
	// identifier swap, so this should not be treated as a rename even
	// though the line count lines up.
	hunk := Hunk{
		NewStart: 5,
		Content:  "-func RunHelp(oldParam string) {\n+func RuHelp(newParam string) {",
	}
	sym := symbols.Symbol{Name: "RuHelp", StartLine: 5, EndLine: 8}

	if rn := detectDeclRename(hunk, sym); rn != nil {
		t.Fatalf("expected no rename detected for a multi-token change, got %+v", rn)
	}
}
