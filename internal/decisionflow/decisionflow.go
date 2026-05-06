package decisionflow

import (
	"net/http"
	"strings"
)

const (
	DecisionCommit  = 0 // proceed with commit
	DecisionAbort   = 1 // abort commit
	DecisionSkip    = 2 // skip review, proceed with commit
	DecisionVouch   = 4 // vouch for changes, proceed with commit
	DecisionHandoff = 5 // handoff to AI agent
)

type Phase int

const (
	PhaseReviewRunning Phase = iota
	PhaseReviewComplete
)

func ActionAllowedInPhase(code int, phase Phase) bool {
	switch code {
	case DecisionAbort:
		return true
	case DecisionSkip, DecisionVouch:
		return phase == PhaseReviewRunning
	case DecisionCommit, DecisionHandoff:
		return phase == PhaseReviewComplete
	default:
		return false
	}
}

type RequestError struct {
	status  int
	message string
}

func (e *RequestError) Error() string {
	return e.message
}

func (e *RequestError) StatusCode() int {
	return e.status
}

func ValidateRequest(code int, message string, phase Phase) error {
	if !ActionAllowedInPhase(code, phase) {
		return &RequestError{status: http.StatusConflict, message: "action not allowed in current review stage"}
	}
	if code == DecisionCommit && strings.TrimSpace(message) == "" {
		return &RequestError{status: http.StatusBadRequest, message: "commit message is required in web UI"}
	}
	return nil
}
