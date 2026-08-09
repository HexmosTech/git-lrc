package appcore

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewdb"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/syncqueue"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/gofrs/flock"
	"github.com/urfave/cli/v2"
)

// syncFlushLockPath is the flock single-flight lock so concurrent commits
// (across repos) or overlapping opportunistic triggers don't spawn a pile
// of redundant background flush workers.
func syncFlushLockPath() (string, error) {
	dataDir, err := configpath.ResolveLRCDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "sync-flush.lock"), nil
}

// RunSyncEnqueue implements `lrc internal sync enqueue`, called from
// hooks/post-commit.sh right after every commit. It must never fail the
// hook: all errors are swallowed (optionally logged with --verbose) since
// the commit has already happened by the time this runs.
func RunSyncEnqueue(c *cli.Context) error {
	verbose := c.Bool("verbose")

	commitSHA, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		logSyncWarn(verbose, "could not resolve HEAD: %v", err)
		return nil
	}
	treeHash, err := gitOutput("rev-parse", "HEAD^{tree}")
	if err != nil {
		logSyncWarn(verbose, "could not resolve HEAD's tree: %v", err)
		return nil
	}

	candidate, found, err := reviewdb.GetSyncCandidateForTreeHash(treeHash)
	if err != nil {
		logSyncWarn(verbose, "could not look up review session for tree %s: %v", treeHash, err)
		return nil
	}
	if !found {
		// Nothing to sync: "skipped", or a ledger cleared before this
		// commit existed. Not an error.
		return nil
	}

	repoPath, err := reviewapi.ResolveRepoRoot()
	if err != nil {
		repoPath = ""
	}
	remoteURL, _ := gitOutput("remote", "get-url", "origin") // best-effort, informational only

	db, err := syncqueue.Open()
	if err != nil {
		logSyncWarn(verbose, "could not open sync queue: %v", err)
		return nil
	}
	defer db.Close()

	err = syncqueue.Enqueue(db, syncqueue.EnqueueInput{
		RepoPath:  repoPath,
		RemoteURL: remoteURL,
		Branch:    candidate.Branch,
		CommitSHA: commitSHA,
		TreeHash:  treeHash,
		ReviewID:  candidate.ReviewID,
		APIURL:    candidate.APIURL,
		APIKey:    candidate.APIKey,
	})
	if err != nil {
		logSyncWarn(verbose, "could not enqueue commit sync: %v", err)
		return nil
	}
	if verbose {
		fmt.Printf("lrc sync: queued commit %s (review %s) for background sync\n", shortSHA(commitSHA), candidate.ReviewID)
	}

	spawnDetachedFlushWorker(verbose)
	return nil
}

// gitOutput runs a git command in the current directory and returns its
// trimmed stdout. Thin wrapper so RunSyncEnqueue reads cleanly.
func gitOutput(args ...string) (string, error) {
	out, err := reviewapi.RunGitCommand(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func logSyncWarn(verbose bool, format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, "lrc sync: warning: "+format+"\n", args...)
	}
}

// spawnDetachedFlushWorker fire-and-forgets `lrc internal sync flush-worker`
// as a fully detached child process (mirrors claude_setup.go's worker spawn:
// devNull stdio, session-detached via setupSessionDetach, Start()+reaping
// goroutine) so it keeps running after the calling command exits, and never
// adds network latency to the caller (post-commit hook or any other `lrc`
// invocation). The worker re-checks the single-flight lock itself, so it's
// always safe to call this even if a worker might already be running.
func spawnDetachedFlushWorker(verbose bool) {
	lrcExe, err := os.Executable()
	if err != nil {
		logSyncWarn(verbose, "cannot find lrc executable to spawn sync worker: %v", err)
		return
	}

	devNull, _ := storage.OpenFile(os.DevNull, os.O_RDWR, 0)
	// Safe: re-invokes this same lrc binary (os.Executable) with a fixed subcommand.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.Command(lrcExe, "internal", "sync", "flush-worker")
	if devNull != nil {
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}
	setupSessionDetach(cmd)

	if err := cmd.Start(); err != nil {
		if devNull != nil {
			devNull.Close()
		}
		logSyncWarn(verbose, "failed to spawn sync flush worker: %v", err)
		return
	}
	go func() {
		_ = cmd.Wait()
		if devNull != nil {
			devNull.Close()
		}
	}()
}

// RunSyncFlushWorker implements `lrc internal sync flush-worker`: the
// detached background process spawned by spawnDetachedFlushWorker. Acquires
// the single-flight lock itself (TryLock, non-blocking) and exits quietly
// if another worker already holds it.
func RunSyncFlushWorker(c *cli.Context) error {
	_, err := tryFlush(c.Bool("verbose"))
	return err
}

// RunSyncFlush implements `lrc sync flush`: a synchronous, foreground,
// user-requested flush (e.g. "I just got back online"). Reports whether it
// actually ran (vs. a background worker already holding the lock) rather
// than blocking/waiting for one.
func RunSyncFlush(c *cli.Context) error {
	fmt.Println("Flushing pending review-commit sync items...")
	locked, err := tryFlush(true)
	if err != nil {
		return fmt.Errorf("sync flush failed: %w", err)
	}
	if !locked {
		fmt.Println("A sync is already in progress in the background; check `lrc sync status` shortly.")
		return nil
	}
	return RunSyncStatus(c)
}

// tryFlush acquires the single-flight lock (non-blocking) and, if
// successful, runs one flush pass. locked=false means another flush
// (worker or foreground) already holds the lock -- not an error.
func tryFlush(verbose bool) (locked bool, err error) {
	lockPath, err := syncFlushLockPath()
	if err != nil {
		return false, err
	}
	fl := flock.New(lockPath)
	got, err := fl.TryLock()
	if err != nil {
		return false, fmt.Errorf("failed to acquire sync flush lock: %w", err)
	}
	if !got {
		return false, nil
	}
	defer fl.Unlock()

	return true, flushSyncQueue(verbose)
}

// flushSyncQueue opens the global sync queue and runs one flush pass
// against it (see flushSyncQueueDB).
func flushSyncQueue(verbose bool) error {
	db, err := syncqueue.Open()
	if err != nil {
		return err
	}
	defer db.Close()

	return flushSyncQueueDB(db, verbose)
}

// flushSyncQueueDB POSTs every due pending item in db to its (per-item,
// snapshotted) api_url/api_key, then records the outcome. 401/403/404 are
// treated as permanent (wrong/revoked key, or a review that doesn't belong
// to that org) and are never retried; anything else backs off (see
// syncqueue.RecordFailure). Split out from flushSyncQueue so it's testable
// against an in-memory queue DB and a mock server, without touching the
// real ~/.lrc/sync-queue.db.
func flushSyncQueueDB(db *sql.DB, verbose bool) error {
	items, err := syncqueue.Due(db, time.Now())
	if err != nil {
		return err
	}
	if len(items) == 0 {
		if verbose {
			fmt.Println("lrc sync: nothing due")
		}
		return nil
	}

	client := network.NewReviewAPIClient(10 * time.Second)
	for _, item := range items {
		resp, reqErr := network.ReviewAttachCommit(client, item.APIURL, item.ReviewID, reviewmodel.AttachCommitRequest{CommitSHA: item.CommitSHA}, item.APIKey)
		now := time.Now()
		if reqErr != nil {
			_ = syncqueue.RecordFailure(db, item.ID, false, reqErr.Error(), now, item.APIKey)
			if verbose {
				fmt.Printf("lrc sync: #%d commit %s: transient failure: %v\n", item.ID, shortSHA(item.CommitSHA), reqErr)
			}
			continue
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			_ = syncqueue.MarkSynced(db, item.ID, now)
			if verbose {
				fmt.Printf("lrc sync: #%d commit %s: synced\n", item.ID, shortSHA(item.CommitSHA))
			}
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
			errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
			_ = syncqueue.RecordFailure(db, item.ID, true, errMsg, now, item.APIKey)
			if verbose {
				fmt.Printf("lrc sync: #%d commit %s: permanent failure (%s) -- will not retry\n", item.ID, shortSHA(item.CommitSHA), errMsg)
			}
		default:
			errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
			_ = syncqueue.RecordFailure(db, item.ID, false, errMsg, now, item.APIKey)
			if verbose {
				fmt.Printf("lrc sync: #%d commit %s: transient failure (%s)\n", item.ID, shortSHA(item.CommitSHA), errMsg)
			}
		}
	}
	return nil
}

// RunSyncStatus implements `lrc sync status`.
func RunSyncStatus(c *cli.Context) error {
	db, err := syncqueue.Open()
	if err != nil {
		return err
	}
	defer db.Close()

	stats, err := syncqueue.GetStats(db)
	if err != nil {
		return err
	}

	fmt.Printf("Pending: %d\n", stats.Pending)
	fmt.Printf("Synced:  %d\n", stats.Synced)
	fmt.Printf("Failed:  %d\n", stats.Failed)
	if stats.OldestPending != nil {
		fmt.Printf("Oldest pending item: %s\n", formatAge(time.Since(*stats.OldestPending)))
	}
	if stats.LastAttemptAt != nil {
		fmt.Printf("Last sync attempt: %s\n", formatAge(time.Since(*stats.LastAttemptAt)))
	}
	if stats.Failed > 0 {
		fmt.Println("\nSome items stopped retrying -- see `lrc sync list --status=failed` for details.")
	}
	return nil
}

// RunSyncList implements `lrc sync list [--status=pending|failed|synced]`.
func RunSyncList(c *cli.Context) error {
	status := c.String("status")

	db, err := syncqueue.Open()
	if err != nil {
		return err
	}
	defer db.Close()

	items, err := syncqueue.List(db, status)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("No sync queue items.")
		return nil
	}

	for _, it := range items {
		fmt.Printf("#%-4d %-9s attempts=%-2d age=%-10s repo=%s commit=%s review=%s\n",
			it.ID, it.Status, it.Attempts, formatAge(time.Since(it.CreatedAt)),
			filepath.Base(it.RepoPath), shortSHA(it.CommitSHA), it.ReviewID)
		if it.LastError != "" {
			fmt.Printf("       last_error: %s\n", it.LastError)
		}
	}
	return nil
}

// RunSyncForget implements `lrc sync forget <id>`.
func RunSyncForget(c *cli.Context) error {
	idStr := strings.TrimSpace(c.Args().First())
	if idStr == "" {
		return fmt.Errorf("usage: lrc sync forget <id> (see `lrc sync list` for ids)")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", idStr, err)
	}

	db, err := syncqueue.Open()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := syncqueue.Forget(db, id); err != nil {
		return err
	}
	fmt.Printf("Forgot sync queue item #%d\n", id)
	return nil
}

// TriggerOpportunisticSyncFlush is called unconditionally on every `lrc`
// invocation (see main.go, next to selfupdate.EnsureGitLRCBinarySynced) so
// pending items eventually sync even without a post-commit hook firing
// again -- e.g. after being offline for days, the very next `lrc` command
// run anywhere (any repo) picks the global queue back up. Deliberately
// cheap: a single indexed COUNT query, and only spawns a worker (never
// flushes inline) when there's actually something due.
func TriggerOpportunisticSyncFlush() {
	// No --verbose flag reaches this (it runs before command dispatch, on
	// every invocation), so LRC_VERBOSE is the only way to see these --
	// same escape hatch documented on the `verbose` flag definitions.
	verbose := os.Getenv("LRC_VERBOSE") != ""

	db, err := syncqueue.Open()
	if err != nil {
		logSyncWarn(verbose, "opportunistic sync check: could not open sync queue: %v", err)
		return
	}
	defer db.Close()

	count, err := syncqueue.CountDue(db, time.Now())
	if err != nil {
		logSyncWarn(verbose, "opportunistic sync check: could not count due items: %v", err)
		return
	}
	if count == 0 {
		return
	}
	spawnDetachedFlushWorker(verbose)
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
