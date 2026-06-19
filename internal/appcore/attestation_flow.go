package appcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewdb"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/urfave/cli/v2"
)

type attestationPayload struct {
	Action           string  `json:"action"`
	Iterations       int     `json:"iterations"`
	PriorAICovPct    float64 `json:"prior_ai_coverage_pct"`
	PriorReviewCount int     `json:"prior_review_count"`
}

// pushAttestationTip, when set, redirects attestation reads/writes for the current
// process to a push attestation (keyed by the push's tip commit SHA, stored under
// lrc/push_attestations/) instead of the default tree-hash attestation used for
// staged-change commit gating. Set once via setPushAttestationTip at the start of a
// --push-range review; a single CLI invocation never reviews more than one tip.
var pushAttestationTip string

func setPushAttestationTip(tip string) {
	pushAttestationTip = tip
}

// attestationKeyAndDir resolves the identifier and directory used for the current
// attestation: the push tip SHA under lrc/push_attestations/ in push mode, or the
// staged tree hash under lrc/attestations/ otherwise.
func attestationKeyAndDir() (key, dir string, err error) {
	gitDir, err := reviewapi.ResolveGitDir()
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(gitDir) {
		if abs, absErr := filepath.Abs(gitDir); absErr == nil {
			gitDir = abs
		}
	}

	if pushAttestationTip != "" {
		return pushAttestationTip, filepath.Join(gitDir, "lrc", "push_attestations"), nil
	}

	treeHash, err := reviewapi.CurrentTreeHash()
	if err != nil {
		return "", "", err
	}
	return treeHash, filepath.Join(gitDir, "lrc", "attestations"), nil
}

func ensureAttestation(action string, verbose bool, written *bool) error {
	return ensureAttestationFull(attestationPayload{Action: action}, verbose, written)
}

// recordCoverageAndAttest parses the diff, records a review session with coverage stats,
// and writes a full attestation. Used by both the "reviewed" and "vouched" interactive paths.
func recordCoverageAndAttest(action string, diffContent []byte, reviewID string, verbose bool, attestationWritten *bool) error {
	parsedFiles, parseErr := parseDiffToFiles(diffContent)
	if parseErr != nil {
		return fmt.Errorf("could not parse diff for coverage tracking: %w", parseErr)
	}
	cov, covErr := reviewdb.RecordAndComputeCoverage(action, parsedFiles, reviewID, verbose)
	if covErr != nil {
		return fmt.Errorf("coverage computation failed: %w", covErr)
	}
	if cov.Iterations == 0 {
		cov.Iterations = 1
	}
	return ensureAttestationFull(attestationPayload{
		Action:           action,
		Iterations:       cov.Iterations,
		PriorAICovPct:    cov.PriorAICovPct,
		PriorReviewCount: cov.PriorReviewCount,
	}, verbose, attestationWritten)
}

func ensureAttestationFull(payload attestationPayload, verbose bool, written *bool) error {
	if written != nil && *written {
		return nil
	}
	if strings.TrimSpace(payload.Action) == "" {
		return nil
	}

	path, err := writeAttestationFullForCurrentTree(payload)
	if err != nil {
		return fmt.Errorf("failed to write attestation: %w", err)
	}
	if verbose {
		log.Printf("Attestation written: %s (action=%s, iter:%d, coverage:%.0f%%)",
			path, payload.Action, payload.Iterations, payload.PriorAICovPct)
	}
	if written != nil {
		*written = true
	}
	return nil
}

// existingAttestationAction returns the attestation action for the current tree (or push
// tip, in push mode), if present.
func existingAttestationAction() (string, error) {
	key, dir, err := attestationKeyAndDir()
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", nil
	}

	attestPath := filepath.Join(dir, fmt.Sprintf("%s.json", key))
	data, err := storage.ReadAttestationFile(attestPath)
	if err != nil {
		return "", nil
	}

	var payload attestationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", nil
	}

	return strings.TrimSpace(payload.Action), nil
}

// readCurrentAttestation reads and parses the full attestation payload for the current
// tree (or push tip, in push mode).
func readCurrentAttestation() (*attestationPayload, error) {
	key, dir, err := attestationKeyAndDir()
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, nil
	}

	attestPath := filepath.Join(dir, fmt.Sprintf("%s.json", key))
	data, err := storage.ReadAttestationFile(attestPath)
	if err != nil {
		return nil, nil
	}

	var payload attestationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("malformed attestation JSON: %w", err)
	}

	return &payload, nil
}

// runAttestationTrailer outputs the formatted commit trailer from the current
// attestation. Called by the commit-msg hook to avoid fragile sed JSON parsing.
// Outputs nothing (and exits 0) if no attestation is present.
func runAttestationTrailer(c *cli.Context) error {
	payload, err := readCurrentAttestation()
	if err != nil {
		return err
	}
	if payload == nil || strings.TrimSpace(payload.Action) == "" {
		return nil
	}

	var trailerVal string
	switch payload.Action {
	case "reviewed":
		trailerVal = "ran"
	case "skipped":
		trailerVal = "skipped"
	case "vouched":
		trailerVal = "vouched"
	default:
		trailerVal = payload.Action
	}

	if payload.Iterations > 0 {
		covPct := int(payload.PriorAICovPct + 0.5)
		trailerVal = fmt.Sprintf("%s (iter:%d, coverage:%d%%)", trailerVal, payload.Iterations, covPct)
	}

	fmt.Printf("LiveReview Pre-Commit Check: %s", trailerVal)
	return nil
}

func writeAttestationForCurrentTree(action string) (string, error) {
	return writeAttestationFullForCurrentTree(attestationPayload{Action: action})
}

func writeAttestationFullForCurrentTree(payload attestationPayload) (string, error) {
	if strings.TrimSpace(payload.Action) == "" {
		return "", fmt.Errorf("attestation action cannot be empty")
	}

	key, attestDir, err := attestationKeyAndDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve attestation key: %w", err)
	}
	if key == "" {
		return "", fmt.Errorf("empty attestation key")
	}

	if err := storage.EnsureAttestationOutputDir(attestDir); err != nil {
		return "", fmt.Errorf("failed to create attestation directory: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal attestation: %w", err)
	}

	target := filepath.Join(attestDir, fmt.Sprintf("%s.json", key))
	if err := storage.WriteFileAtomically(target, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write attestation: %w", err)
	}

	return target, nil
}

// RunRemoveAttestation removes the attestation for the current staged tree, if present.
func RunRemoveAttestation(c *cli.Context) error {
	action, err := existingAttestationAction()
	if err != nil {
		return fmt.Errorf("could not read attestation: %w", err)
	}
	if action == "" {
		fmt.Println("LiveReview: no attestation found for current tree")
		return nil
	}
	if err := deleteAttestationForCurrentTree(); err != nil {
		return err
	}
	fmt.Printf("LiveReview: attestation removed (was: %s)\n", action)
	return nil
}

func deleteAttestationForCurrentTree() error {
	key, dir, err := attestationKeyAndDir()
	if err != nil {
		return fmt.Errorf("failed to resolve attestation key: %w", err)
	}
	if key == "" {
		return nil
	}

	attestPath := filepath.Join(dir, fmt.Sprintf("%s.json", key))
	if err := storage.RemoveAttestationFile(attestPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to delete attestation %s: %w", attestPath, err)
	}

	return nil
}
