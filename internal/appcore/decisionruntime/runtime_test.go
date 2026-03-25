package decisionruntime

import (
	"errors"
	"testing"

	"github.com/HexmosTech/git-lrc/internal/decisionflow"
)

func TestRuntimeFirstValidDecisionWins(t *testing.T) {
	rt := New(decisionflow.PhaseReviewRunning)

	first := rt.TryDecide(Decision{Source: SourceTerminal, Code: decisionflow.DecisionSkip})
	if !first.Accepted || first.Err != nil {
		t.Fatalf("first decision should be accepted, got accepted=%v err=%v", first.Accepted, first.Err)
	}

	second := rt.TryDecide(Decision{Source: SourceWeb, Code: decisionflow.DecisionAbort})
	if second.Accepted {
		t.Fatalf("second decision should not be accepted")
	}
	if !errors.Is(second.Err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved, got %v", second.Err)
	}

	chosen, ok := rt.Chosen()
	if !ok {
		t.Fatalf("expected chosen decision to be present")
	}
	if chosen.Code != decisionflow.DecisionSkip || chosen.Source != SourceTerminal {
		t.Fatalf("unexpected chosen decision: %+v", chosen)
	}
}

func TestRuntimePhaseGateAndWebValidation(t *testing.T) {
	rt := New(decisionflow.PhaseReviewRunning)

	invalid := rt.TryDecide(Decision{Source: SourceTerminal, Code: decisionflow.DecisionCommit})
	if invalid.Accepted {
		t.Fatalf("terminal commit should be rejected while review is running")
	}
	if !errors.Is(invalid.Err, ErrActionNotAllowed) {
		t.Fatalf("expected ErrActionNotAllowed, got %v", invalid.Err)
	}

	rt.SetPhase(decisionflow.PhaseReviewComplete)

	webInvalid := rt.TryDecide(Decision{Source: SourceWeb, Code: decisionflow.DecisionCommit, Message: "   "})
	if webInvalid.Accepted {
		t.Fatalf("web commit with empty message should be rejected")
	}
	reqErr, ok := webInvalid.Err.(*decisionflow.RequestError)
	if !ok {
		t.Fatalf("expected *decisionflow.RequestError, got %T", webInvalid.Err)
	}
	if reqErr.StatusCode() != 400 {
		t.Fatalf("status = %d, want 400", reqErr.StatusCode())
	}

	commit := rt.TryDecide(Decision{Source: SourceTerminal, Code: decisionflow.DecisionCommit})
	if !commit.Accepted || commit.Err != nil {
		t.Fatalf("terminal commit should be accepted in complete phase, err=%v", commit.Err)
	}
}

func TestResolveFinalMessage(t *testing.T) {
	tests := []struct {
		name    string
		d       Decision
		initial string
		editor  string
		want    string
	}{
		{name: "web message wins", d: Decision{Source: SourceWeb, Message: "web"}, initial: "cli", editor: "editor", want: "web"},
		{name: "editor fallback", d: Decision{Source: SourceTerminal, Message: ""}, initial: "cli", editor: "editor", want: "editor"},
		{name: "event fallback", d: Decision{Source: SourceTerminal, Message: "terminal"}, initial: "cli", editor: "", want: "terminal"},
		{name: "initial fallback", d: Decision{Source: SourceTerminal, Message: ""}, initial: "cli", editor: "", want: "cli"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveFinalMessage(tt.d, tt.initial, tt.editor)
			if got != tt.want {
				t.Fatalf("ResolveFinalMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeWebSkipVouchAllowedAfterReviewComplete(t *testing.T) {
	t.Run("web skip allowed", func(t *testing.T) {
		rt := New(decisionflow.PhaseReviewComplete)
		outcome := rt.TryDecide(Decision{Source: SourceWeb, Code: decisionflow.DecisionSkip})
		if !outcome.Accepted || outcome.Err != nil {
			t.Fatalf("expected web skip to be accepted after review complete, got accepted=%v err=%v", outcome.Accepted, outcome.Err)
		}
	})

	t.Run("web vouch allowed", func(t *testing.T) {
		rt := New(decisionflow.PhaseReviewComplete)
		outcome := rt.TryDecide(Decision{Source: SourceWeb, Code: decisionflow.DecisionVouch})
		if !outcome.Accepted || outcome.Err != nil {
			t.Fatalf("expected web vouch to be accepted after review complete, got accepted=%v err=%v", outcome.Accepted, outcome.Err)
		}
	})

	t.Run("terminal skip allowed", func(t *testing.T) {
		rt := New(decisionflow.PhaseReviewComplete)
		outcome := rt.TryDecide(Decision{Source: SourceTerminal, Code: decisionflow.DecisionSkip})
		if !outcome.Accepted || outcome.Err != nil {
			t.Fatalf("expected terminal skip to be accepted after review complete, got accepted=%v err=%v", outcome.Accepted, outcome.Err)
		}
	})

	t.Run("terminal vouch allowed", func(t *testing.T) {
		rt := New(decisionflow.PhaseReviewComplete)
		outcome := rt.TryDecide(Decision{Source: SourceTerminal, Code: decisionflow.DecisionVouch})
		if !outcome.Accepted || outcome.Err != nil {
			t.Fatalf("expected terminal vouch to be accepted after review complete, got accepted=%v err=%v", outcome.Accepted, outcome.Err)
		}
	})
}
