package decisionruntime

import (
	"errors"
	"sync"

	"github.com/HexmosTech/git-lrc/internal/decisionflow"
)

// Source identifies where a decision originated.
type Source string

const (
	SourceTerminal Source = "terminal"
	SourceWeb      Source = "web"
	SourceSignal   Source = "signal"
)

// Decision is a normalized decision event consumed by runtime arbitration.
type Decision struct {
	Source  Source
	Code    int
	Message string
	Push    bool
}

// Outcome describes the arbitration result for a decision attempt.
type Outcome struct {
	Accepted bool
	Err      error
}

// Runtime arbitrates concurrent decisions and enforces phase constraints.
type Runtime struct {
	mu       sync.RWMutex
	phase    decisionflow.Phase
	resolved bool
	chosen   Decision
}

var (
	ErrAlreadyResolved  = errors.New("decision already resolved")
	ErrActionNotAllowed = errors.New("action not allowed in current review stage")
)

func New(initialPhase decisionflow.Phase) *Runtime {
	return &Runtime{phase: initialPhase}
}

func (r *Runtime) Phase() decisionflow.Phase {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.phase
}

func (r *Runtime) SetPhase(phase decisionflow.Phase) {
	r.mu.Lock()
	r.phase = phase
	r.mu.Unlock()
}

func (r *Runtime) Resolved() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resolved
}

func (r *Runtime) Chosen() (Decision, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.resolved {
		return Decision{}, false
	}
	return r.chosen, true
}

// TryDecide applies first-valid-decision-wins semantics.
func (r *Runtime) TryDecide(d Decision) Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.resolved {
		return Outcome{Accepted: false, Err: ErrAlreadyResolved}
	}

	if r.phase == decisionflow.PhaseReviewComplete && (d.Code == decisionflow.DecisionSkip || d.Code == decisionflow.DecisionVouch) {
		r.resolved = true
		r.chosen = d
		return Outcome{Accepted: true, Err: nil}
	}

	if d.Source == SourceWeb {
		if err := decisionflow.ValidateRequest(d.Code, d.Message, r.phase); err != nil {
			return Outcome{Accepted: false, Err: err}
		}
	} else if !decisionflow.ActionAllowedInPhase(d.Code, r.phase) {
		return Outcome{Accepted: false, Err: ErrActionNotAllowed}
	}

	r.resolved = true
	r.chosen = d
	return Outcome{Accepted: true, Err: nil}
}

// ResolveFinalMessage mirrors the simulator message precedence behavior.
func ResolveFinalMessage(d Decision, initialMessage, editorMessage string) string {
	if d.Source == SourceWeb && d.Message != "" {
		return d.Message
	}
	if editorMessage != "" {
		return editorMessage
	}
	if d.Message != "" {
		return d.Message
	}
	return initialMessage
}
