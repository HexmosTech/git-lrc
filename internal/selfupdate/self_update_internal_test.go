package selfupdate

import (
	"flag"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestInternalBuildDisablesSelfUpdatePaths(t *testing.T) {
	originalVersion := version
	t.Cleanup(func() {
		SetVersion(originalVersion)
	})

	SetVersion("v0.0.0-internal")

	if err := ApplyPendingUpdateIfAny(false); err != nil {
		t.Fatalf("ApplyPendingUpdateIfAny returned error for internal build: %v", err)
	}

	if _, err := stageUpdateVersion("v0.3.4", false, false); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected stageUpdateVersion to fail with disabled error, got: %v", err)
	}

	app := cli.NewApp()
	set := flag.NewFlagSet("self-update", flag.ContinueOnError)
	set.Bool("check", false, "")
	set.Bool("force", false, "")
	ctx := cli.NewContext(app, set, nil)

	if err := RunSelfUpdate(ctx); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected RunSelfUpdate to fail with disabled error, got: %v", err)
	}
}
