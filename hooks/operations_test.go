package hooks

import (
	"strings"
	"testing"
)

func TestReplaceManagedSection(t *testing.T) {
	content := "#!/bin/sh\n" +
		"echo before\n" +
		"# BEGIN lrc managed section - DO NOT EDIT\n" +
		"old\n" +
		"# END lrc managed section\n" +
		"echo after\n"

	updated := ReplaceManagedSection(content, "new", "# BEGIN lrc managed section - DO NOT EDIT", "# END lrc managed section")

	if updated == content {
		t.Fatalf("expected section replacement")
	}
	if !strings.Contains(updated, "new") {
		t.Fatalf("expected new managed section content")
	}
	if strings.Contains(updated, "old") {
		t.Fatalf("did not expect old managed section content")
	}
}

func TestRemoveManagedSection(t *testing.T) {
	content := "#!/bin/sh\n" +
		"echo before\n" +
		"# BEGIN lrc managed section - DO NOT EDIT\n" +
		"managed\n" +
		"# END lrc managed section\n" +
		"echo after\n"

	updated := RemoveManagedSection(content, "# BEGIN lrc managed section - DO NOT EDIT", "# END lrc managed section")

	if strings.Contains(updated, "managed") {
		t.Fatalf("managed block should be removed")
	}
	if !strings.Contains(updated, "echo before") || !strings.Contains(updated, "echo after") {
		t.Fatalf("expected non-managed content to remain")
	}
}
