package appui

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

func ensureReviewReady(selectedAPIURL string) error {
	slog := newSetupLog()
	defer cleanupSetupLog(slog)

	preflight, err := runSetupPreflight(resolveSetupTargetAPIURL(selectedAPIURL), slog)
	if err != nil {
		return fmt.Errorf("LiveReview readiness check failed: %w\n\nIf setup or authentication changed, run:\n  1. lrc setup\n  2. or lrc ui\n  3. then retry your review command", err)
	}
	if preflight == nil || !preflight.AuthReady {
		return fmt.Errorf("LiveReview setup is required before AI review can run.\n\nNext steps:\n  1. Run: lrc setup\n  2. Or run: lrc ui\n  3. Retry your review command")
	}
	if !preflight.HasConnector {
		label := existingSetupLabel(preflight)
		if strings.TrimSpace(label) == "" {
			label = "your account"
		}
		return fmt.Errorf("LiveReview is authenticated for %s, but no AI connector is configured.\n\nNext steps:\n  1. Run: lrc setup\n  2. Or run: lrc ui to add a connector\n  3. Retry your review command", label)
	}

	return nil
}

// RunEnsure checks whether AI review readiness is satisfied without mutating setup state.
func RunEnsure(c *cli.Context) error {
	return ensureReviewReady(c.String("api-url"))
}
