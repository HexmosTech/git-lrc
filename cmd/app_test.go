package cmd

import "testing"

func TestSetupCommandIncludesAPIURLChoiceFlags(t *testing.T) {
	app := BuildApp("dev", "now", "none", "prod", nil, nil, Handlers{})

	var setupCommandFound bool
	var setupCommandFlags map[string]bool

	for _, command := range app.Commands {
		if command.Name != "setup" {
			continue
		}
		setupCommandFound = true
		setupCommandFlags = map[string]bool{}
		for _, flag := range command.Flags {
			for _, name := range flag.Names() {
				setupCommandFlags[name] = true
			}
		}
		break
	}

	if !setupCommandFound {
		t.Fatalf("setup command not found")
	}

	for _, expected := range []string{"api-url", "base-url", "yes", "keep-api-url", "replace-api-url"} {
		if !setupCommandFlags[expected] {
			t.Fatalf("setup command missing flag %q", expected)
		}
	}
}
