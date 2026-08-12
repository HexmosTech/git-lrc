package terminology

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shrsv/dbctx/internal/db"
)

// Import validates and persists a JSON array of [TermGroup] into store.
// Every (alias, target) pair is validated independently: a malformed or
// unresolvable entry is recorded in the result's Rejected list rather than
// aborting the whole import, so one bad line doesn't lose an otherwise
// good batch. Nothing is written for a term/alias/target combination that
// fails validation.
//
// Import is idempotent for identical (alias, target) pairs — re-importing
// the same mapping updates its term/source/imported_at rather than
// duplicating it (see the unique index in InitTerminologySchema).
//
// Import calls store.InitTerminologySchema itself, so it works against a
// .dtx file that predates terminology support without any separate
// migration step.
func Import(store *db.Store, data []byte) (*ImportResult, error) {
	if err := store.InitTerminologySchema(); err != nil {
		return nil, fmt.Errorf("init terminology schema: %w", err)
	}

	var groups []TermGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("parse terminology JSON: %w", err)
	}

	result := &ImportResult{}
	now := time.Now().UTC().Format(time.RFC3339)

	for _, g := range groups {
		term := strings.TrimSpace(g.Term)
		if term == "" {
			result.Rejected = append(result.Rejected, RejectedEntry{
				Term: g.Term, Reason: "term is empty",
			})
			continue
		}
		if len(g.Aliases) == 0 {
			result.Rejected = append(result.Rejected, RejectedEntry{
				Term: term, Reason: "no aliases",
			})
			continue
		}
		if len(g.Targets) == 0 {
			result.Rejected = append(result.Rejected, RejectedEntry{
				Term: term, Reason: "no targets",
			})
			continue
		}

		seenAlias := make(map[string]bool)
		for _, rawAlias := range g.Aliases {
			alias := strings.TrimSpace(rawAlias)
			if alias == "" {
				result.Rejected = append(result.Rejected, RejectedEntry{
					Term: term, Alias: rawAlias, Reason: "alias is empty",
				})
				continue
			}
			lowerAlias := strings.ToLower(alias)
			if seenAlias[lowerAlias] {
				result.Rejected = append(result.Rejected, RejectedEntry{
					Term: term, Alias: alias, Reason: "duplicate alias within this term (kept the first occurrence)",
				})
				continue
			}
			seenAlias[lowerAlias] = true

			for _, rawTarget := range g.Targets {
				table, column, path, err := parseTarget(rawTarget)
				if err != nil {
					result.Rejected = append(result.Rejected, RejectedEntry{
						Term: term, Alias: alias, Target: rawTarget, Reason: err.Error(),
					})
					continue
				}
				if err := validateTarget(store, table, column, path); err != nil {
					result.Rejected = append(result.Rejected, RejectedEntry{
						Term: term, Alias: alias, Target: rawTarget, Reason: err.Error(),
					})
					continue
				}

				_, err = store.DB().Exec(`
					INSERT INTO terminology (term, alias, target_table, target_column, target_path, source, imported_at)
					VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), 'user', ?)
					ON CONFLICT(alias, target_table, IFNULL(target_column,''), IFNULL(target_path,''))
					DO UPDATE SET term = excluded.term, imported_at = excluded.imported_at
				`, term, alias, table, column, path, now)
				if err != nil {
					result.Rejected = append(result.Rejected, RejectedEntry{
						Term: term, Alias: alias, Target: rawTarget, Reason: fmt.Sprintf("write failed: %v", err),
					})
					continue
				}
				result.Accepted++
			}
		}
	}

	return result, nil
}

// List returns every persisted terminology entry, ordered by term then
// alias, for inspection — "imported terminology should be inspectable."
// Returns an empty (not nil-error) slice if the store has no terminology
// table yet (terminology was never imported).
func List(store *db.Store) ([]Entry, error) {
	var count int
	if err := store.DB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='terminology'",
	).Scan(&count); err != nil || count == 0 {
		return nil, nil
	}

	rows, err := store.DB().Query(`
		SELECT id, term, alias, target_table, COALESCE(target_column, ''), COALESCE(target_path, ''),
		       source, COALESCE(imported_at, '')
		FROM terminology
		ORDER BY term, alias
	`)
	if err != nil {
		return nil, fmt.Errorf("list terminology: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Term, &e.Alias, &e.TargetTable, &e.TargetColumn, &e.TargetPath, &e.Source, &e.ImportedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
