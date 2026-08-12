package semantic

import (
	"strings"

	"github.com/shrsv/dbctx/internal/db"
)

type tableRow struct {
	id   int64
	name string
}

type columnRow struct {
	id            int64
	name          string
	dataType      string
	isStateLike   bool
	isCategorical bool
	isFK          bool
	fkTarget      string
}

type jsonbPathRow struct {
	id           int64
	path         string
	inferredType string
	sampleValues string
}

func loadTables(store *db.Store) ([]tableRow, error) {
	rows, err := store.DB().Query("SELECT id, name FROM tables ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []tableRow
	for rows.Next() {
		var t tableRow
		if err := rows.Scan(&t.id, &t.name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadColumns(store *db.Store, tableID int64) ([]columnRow, error) {
	rows, err := store.DB().Query(`
		SELECT c.id, c.name, c.type,
		       COALESCE(fs.is_state_like, 0), COALESCE(fs.is_categorical, 0)
		FROM columns c
		LEFT JOIN field_stats fs ON fs.column_id = c.id
		WHERE c.table_id = ?
		ORDER BY c.position
	`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []columnRow
	for rows.Next() {
		var c columnRow
		var isState, isCat int
		if err := rows.Scan(&c.id, &c.name, &c.dataType, &isState, &isCat); err != nil {
			return nil, err
		}
		c.isStateLike = isState == 1
		c.isCategorical = isCat == 1
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fkRows, err := store.DB().Query(`
		SELECT fk.src_columns, rt.name, fk.dst_columns
		FROM foreign_keys fk
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE fk.table_id = ?
	`, tableID)
	if err != nil {
		return nil, err
	}
	defer fkRows.Close()

	fkTargetByCol := make(map[string]string)
	for fkRows.Next() {
		var srcCols, refTable, dstCols string
		if err := fkRows.Scan(&srcCols, &refTable, &dstCols); err != nil {
			return nil, err
		}
		target := refTable
		if dstCols != "id" {
			target += "." + dstCols
		}
		for _, col := range strings.Split(srcCols, ",") {
			fkTargetByCol[strings.TrimSpace(col)] = target
		}
	}
	if err := fkRows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if target, ok := fkTargetByCol[out[i].name]; ok {
			out[i].isFK = true
			out[i].fkTarget = target
		}
	}
	return out, nil
}

func loadFKTargets(store *db.Store, tableID int64) ([]string, error) {
	rows, err := store.DB().Query(`
		SELECT DISTINCT rt.name
		FROM foreign_keys fk
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE fk.table_id = ?
	`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// loadTableValues returns representative values across all state/categorical
// columns of a table, ordered by frequency, deduplicated, capped at limit.
func loadTableValues(store *db.Store, tableID int64, limit int) ([]string, error) {
	rows, err := store.DB().Query(`
		SELECT DISTINCT fv.value FROM field_values fv
		JOIN columns c ON c.id = fv.column_id
		JOIN field_stats fs ON fs.column_id = c.id
		WHERE c.table_id = ? AND (fs.is_state_like = 1 OR fs.is_categorical = 1)
		ORDER BY fv.frequency DESC
		LIMIT ?
	`, tableID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func loadColumnValues(store *db.Store, columnID int64, limit int) ([]string, error) {
	rows, err := store.DB().Query(`
		SELECT value FROM field_values
		WHERE column_id = ?
		ORDER BY frequency DESC
		LIMIT ?
	`, columnID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func loadJSONBPaths(store *db.Store, columnID int64, limit int) ([]jsonbPathRow, error) {
	rows, err := store.DB().Query(`
		SELECT id, path, inferred_type, COALESCE(sample_values, '')
		FROM jsonb_paths
		WHERE column_id = ? AND sample_values != ''
		ORDER BY path
		LIMIT ?
	`, columnID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []jsonbPathRow
	for rows.Next() {
		var p jsonbPathRow
		if err := rows.Scan(&p.id, &p.path, &p.inferredType, &p.sampleValues); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
