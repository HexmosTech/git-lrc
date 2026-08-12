package semantic

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/shrsv/dbctx/internal/db"
)

// BuildStats summarizes what a BuildIndex call did, distinguishing objects
// that were actually re-embedded from ones whose text was unchanged and so
// were left alone — the incremental-rebuild behavior the design calls for:
// don't recompute embeddings for schema that hasn't changed.
type BuildStats struct {
	Total    int // candidate objects considered
	Embedded int // newly embedded (new or changed text)
	Reused   int // unchanged since the last build; embedding kept as-is
	Removed  int // stale rows deleted (object no longer exists, e.g. dropped column)
}

// BuildIndex (re)builds the semantic index for store using embedder. It is
// safe to call repeatedly: unchanged objects are left untouched, changed
// objects are re-embedded, and objects that no longer exist in the schema
// are removed. If embedder's model differs from whatever produced the
// currently-persisted embeddings, the entire index is rebuilt from scratch
// (mixing vectors from two different models would make cosine similarity
// meaningless).
//
// log receives human-readable progress messages in the same style as the
// other build phases in dbctx.Build; pass io.Discard to suppress them.
func BuildIndex(store *db.Store, embedder Embedder, log io.Writer) (*BuildStats, error) {
	if log == nil {
		log = io.Discard
	}
	if err := store.InitSemanticSchema(); err != nil {
		return nil, fmt.Errorf("init semantic schema: %w", err)
	}

	if prevModel, _, ok := Available(store); ok && prevModel != embedder.ModelID() {
		fmt.Fprintf(log, "  model changed (%s -> %s); rebuilding semantic index from scratch\n", prevModel, embedder.ModelID())
		if _, err := store.DB().Exec("DELETE FROM semantic_objects"); err != nil {
			return nil, fmt.Errorf("clear stale semantic_objects: %w", err)
		}
	}

	candidates, err := collectCandidates(store)
	if err != nil {
		return nil, fmt.Errorf("collect semantic candidates: %w", err)
	}

	existing, err := loadExistingObjects(store)
	if err != nil {
		return nil, fmt.Errorf("load existing semantic_objects: %w", err)
	}

	stats := &BuildStats{Total: len(candidates)}
	visited := make(map[identity]bool, len(candidates))

	type pending struct {
		candidate  candidate
		existingID int64 // 0 means insert
	}
	var toEmbed []pending

	for _, c := range candidates {
		id := c.identity()
		visited[id] = true
		if ex, ok := existing[id]; ok {
			if ex.textHash == c.hash() {
				stats.Reused++
				continue
			}
			toEmbed = append(toEmbed, pending{c, ex.id})
		} else {
			toEmbed = append(toEmbed, pending{c, 0})
		}
	}

	for id, ex := range existing {
		if !visited[id] {
			if _, err := store.DB().Exec("DELETE FROM semantic_objects WHERE id = ?", ex.id); err != nil {
				return nil, fmt.Errorf("delete stale semantic object %d: %w", ex.id, err)
			}
			stats.Removed++
		}
	}

	if len(toEmbed) > 0 {
		texts := make([]string, len(toEmbed))
		for i, p := range toEmbed {
			texts[i] = p.candidate.Text
		}
		fmt.Fprintf(log, "  embedding %d objects (%d reused, %d removed)...\n", len(toEmbed), stats.Reused, stats.Removed)
		vecs, err := embedder.EmbedPassages(texts)
		if err != nil {
			return nil, fmt.Errorf("embed passages: %w", err)
		}
		if len(vecs) != len(toEmbed) {
			return nil, fmt.Errorf("embedder returned %d vectors for %d texts", len(vecs), len(toEmbed))
		}

		for i, p := range toEmbed {
			blob := encodeVector(vecs[i])
			hash := p.candidate.hash()
			if p.existingID != 0 {
				_, err = store.DB().Exec(
					`UPDATE semantic_objects SET text = ?, text_hash = ?, embedding = ? WHERE id = ?`,
					p.candidate.Text, hash, blob, p.existingID,
				)
			} else {
				c := p.candidate
				_, err = store.DB().Exec(
					`INSERT INTO semantic_objects (kind, table_id, column_id, jsonb_path_id, text, text_hash, embedding)
					 VALUES (?, ?, NULLIF(?, 0), NULLIF(?, 0), ?, ?, ?)`,
					c.Kind, c.TableID, c.ColumnID, c.JSONBPathID, c.Text, hash, blob,
				)
			}
			if err != nil {
				return nil, fmt.Errorf("write semantic object: %w", err)
			}
		}
		stats.Embedded = len(toEmbed)
	}

	if err := setMetadata(store, metaModelKey, embedder.ModelID()); err != nil {
		return nil, err
	}
	if err := setMetadata(store, metaDimsKey, strconv.Itoa(embedder.Dims())); err != nil {
		return nil, err
	}
	if err := setMetadata(store, metaBuiltAtKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}

	return stats, nil
}

type existingObject struct {
	id       int64
	textHash string
}

func loadExistingObjects(store *db.Store) (map[identity]existingObject, error) {
	rows, err := store.DB().Query(`
		SELECT id, kind, table_id, COALESCE(column_id, 0), COALESCE(jsonb_path_id, 0), text_hash
		FROM semantic_objects
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[identity]existingObject)
	for rows.Next() {
		var id identity
		var eo existingObject
		if err := rows.Scan(&eo.id, &id.Kind, &id.TableID, &id.ColumnID, &id.JSONBPathID, &eo.textHash); err != nil {
			return nil, err
		}
		out[id] = eo
	}
	return out, rows.Err()
}

func setMetadata(store *db.Store, key, value string) error {
	_, err := store.DB().Exec("INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)", key, value)
	return err
}

// Available reports whether store has a usable semantic index and, if so,
// the model identity and dimensionality it was built with. Callers should
// check this (cheap: two metadata reads) before constructing a real
// embedder, so opening a .dtx file that was never built with --semantic
// never triggers a model download or load.
func Available(store *db.Store) (modelID string, dims int, ok bool) {
	var m, d string
	if err := store.DB().QueryRow("SELECT value FROM metadata WHERE key = ?", metaModelKey).Scan(&m); err != nil {
		return "", 0, false
	}
	if err := store.DB().QueryRow("SELECT value FROM metadata WHERE key = ?", metaDimsKey).Scan(&d); err != nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(d)
	if err != nil || m == "" || n == 0 {
		return "", 0, false
	}
	return m, n, true
}
