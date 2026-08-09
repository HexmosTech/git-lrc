package syncqueue

import (
	"database/sql"
	"testing"
	"time"

	"github.com/HexmosTech/git-lrc/storage"
)

func openTestQueueDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := storage.ExecSQL(db, schema); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}
	return db
}

func testInput(commitSHA string) EnqueueInput {
	return EnqueueInput{
		RepoPath:  "/repo",
		RemoteURL: "git@example.com:org/repo.git",
		Branch:    "main",
		CommitSHA: commitSHA,
		TreeHash:  "tree-" + commitSHA,
		ReviewID:  "review-1",
		APIURL:    "https://api.example.com",
		APIKey:    "test-key",
	}
}

func TestEnqueueIsIdempotent(t *testing.T) {
	db := openTestQueueDB(t)

	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("second enqueue (same review_id+commit_sha) failed: %v", err)
	}

	items, err := List(db, "")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 item after duplicate enqueue, got %d", len(items))
	}
}

func TestDueAndCountDue(t *testing.T) {
	db := openTestQueueDB(t)
	now := time.Now()

	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	items, err := Due(db, now)
	if err != nil {
		t.Fatalf("Due failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 due item, got %d", len(items))
	}

	count, err := CountDue(db, now)
	if err != nil {
		t.Fatalf("CountDue failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected CountDue=1, got %d", count)
	}

	// A future next_attempt_at must not be "due" yet.
	if err := RecordFailure(db, items[0].ID, false, "network error", now); err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}
	due, err := Due(db, now)
	if err != nil {
		t.Fatalf("Due failed: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected 0 due items right after a transient failure (backoff not elapsed), got %d", len(due))
	}

	// But it becomes due once the backoff window has passed.
	dueLater, err := Due(db, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("Due (later) failed: %v", err)
	}
	if len(dueLater) != 1 {
		t.Fatalf("expected the item due again after backoff elapses, got %d", len(dueLater))
	}
}

func TestMarkSynced(t *testing.T) {
	db := openTestQueueDB(t)
	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	items, _ := List(db, "")
	now := time.Now()

	if err := MarkSynced(db, items[0].ID, now); err != nil {
		t.Fatalf("MarkSynced failed: %v", err)
	}

	updated, _ := List(db, StatusSynced)
	if len(updated) != 1 {
		t.Fatalf("expected 1 synced item, got %d", len(updated))
	}
	if updated[0].SyncedAt == nil {
		t.Error("expected SyncedAt to be set")
	}

	due, _ := Due(db, now)
	if len(due) != 0 {
		t.Error("a synced item must not show up as due")
	}
}

func TestRecordFailure_PermanentStopsRetrying(t *testing.T) {
	db := openTestQueueDB(t)
	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	items, _ := List(db, "")
	now := time.Now()

	if err := RecordFailure(db, items[0].ID, true, "401 unauthorized", now); err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}

	failed, _ := List(db, StatusFailed)
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed item, got %d", len(failed))
	}
	if failed[0].NextAttemptAt != nil {
		t.Error("a permanently-failed item must not have a next_attempt_at")
	}

	// Even far in the future, it must never become "due" again.
	due, _ := Due(db, now.Add(365*24*time.Hour))
	if len(due) != 0 {
		t.Error("a permanently-failed item must never be retried")
	}
}

func TestRecordFailure_TransientBacksOffThenGivesUpAfterMaxAge(t *testing.T) {
	db := openTestQueueDB(t)
	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	items, _ := List(db, "")
	id := items[0].ID
	now := time.Now()

	if err := RecordFailure(db, id, false, "timeout", now); err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}
	stillPending, _ := List(db, StatusPending)
	if len(stillPending) != 1 {
		t.Fatal("expected item to remain pending after one transient failure")
	}
	if stillPending[0].Attempts != 1 {
		t.Errorf("expected attempts=1, got %d", stillPending[0].Attempts)
	}

	// Simulate the same transient failure recurring long after the item was
	// first created -- past MaxPendingAge, it must give up (status=failed).
	longLater := now.Add(MaxPendingAge + time.Hour)
	if err := RecordFailure(db, id, false, "timeout", longLater); err != nil {
		t.Fatalf("RecordFailure (later) failed: %v", err)
	}
	failed, _ := List(db, StatusFailed)
	if len(failed) != 1 {
		t.Fatalf("expected item to be given up on past MaxPendingAge, got status list len=%d", len(failed))
	}
}

func TestForget(t *testing.T) {
	db := openTestQueueDB(t)
	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	items, _ := List(db, "")

	if err := Forget(db, items[0].ID); err != nil {
		t.Fatalf("Forget failed: %v", err)
	}

	remaining, _ := List(db, "")
	if len(remaining) != 0 {
		t.Errorf("expected 0 items after Forget, got %d", len(remaining))
	}
}

// findByCommitSHA looks an item up by CommitSHA rather than relying on
// List's ordering, so tests stay correct even if that ordering changes.
func findByCommitSHA(t *testing.T, items []Item, commitSHA string) Item {
	t.Helper()
	for _, it := range items {
		if it.CommitSHA == commitSHA {
			return it
		}
	}
	t.Fatalf("no item with commit_sha=%q found in %+v", commitSHA, items)
	return Item{}
}

func TestGetStats(t *testing.T) {
	db := openTestQueueDB(t)
	now := time.Now()

	if err := Enqueue(db, testInput("sha1")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if err := Enqueue(db, testInput("sha2")); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	items, _ := List(db, "")
	synced := findByCommitSHA(t, items, "sha2")
	if err := MarkSynced(db, synced.ID, now); err != nil {
		t.Fatalf("MarkSynced failed: %v", err)
	}

	stats, err := GetStats(db)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.Pending != 1 || stats.Synced != 1 || stats.Failed != 0 {
		t.Errorf("unexpected stats: %+v", stats)
	}
	if stats.OldestPending == nil {
		t.Error("expected an oldest-pending timestamp")
	}
}

func TestNextBackoffIsMonotonicAndCapped(t *testing.T) {
	prev := time.Duration(0)
	for attempts := 0; attempts < len(backoffSchedule); attempts++ {
		got := NextBackoff(attempts)
		if got < prev {
			t.Errorf("NextBackoff(%d)=%v is less than NextBackoff(%d)=%v; expected non-decreasing", attempts, got, attempts-1, prev)
		}
		prev = got
	}
	// Beyond the schedule, it must stay capped at the last entry.
	capped := backoffSchedule[len(backoffSchedule)-1]
	if got := NextBackoff(len(backoffSchedule) + 100); got != capped {
		t.Errorf("expected NextBackoff to cap at %v for large attempt counts, got %v", capped, got)
	}
	if got := NextBackoff(-1); got != backoffSchedule[0] {
		t.Errorf("expected NextBackoff(-1) to clamp to the first schedule entry, got %v", got)
	}
}
