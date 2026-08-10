package ui

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/search"
)

type API struct {
	store *db.Store
}

func NewAPI(store *db.Store) *API {
	return &API{store: store}
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", a.handleStats)
	mux.HandleFunc("/api/tables", a.handleTables)
	mux.HandleFunc("/api/tables/", a.handleTableDetail)
	mux.HandleFunc("/api/query", a.handleQuery)
}

func jsonResp(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	var tables, columns, fks, stateFields, catFields, jsonbPaths, fieldValues int
	a.store.DB().QueryRow("SELECT COUNT(*) FROM tables").Scan(&tables)
	a.store.DB().QueryRow("SELECT COUNT(*) FROM columns").Scan(&columns)
	a.store.DB().QueryRow("SELECT COUNT(*) FROM foreign_keys").Scan(&fks)
	a.store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_state_like = 1").Scan(&stateFields)
	a.store.DB().QueryRow("SELECT COUNT(*) FROM field_stats WHERE is_categorical = 1").Scan(&catFields)
	a.store.DB().QueryRow("SELECT COUNT(*) FROM jsonb_paths").Scan(&jsonbPaths)
	a.store.DB().QueryRow("SELECT COUNT(*) FROM field_values").Scan(&fieldValues)
	jsonResp(w, map[string]int{
		"tables": tables, "columns": columns, "foreign_keys": fks,
		"state_fields": stateFields, "categorical_fields": catFields,
		"jsonb_paths": jsonbPaths, "field_values": fieldValues,
	})
}

type tableInfo struct {
	ID          int     `json:"id"`
	Schema      string  `json:"schema"`
	Name        string  `json:"name"`
	RowEstimate float64 `json:"row_estimate"`
	ColCount    int     `json:"columns"`
	FKCount     int     `json:"fk_count"`
}

func (a *API) handleTables(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.DB().Query(`
		SELECT t.id, t.schema, t.name, t.row_estimate,
		       (SELECT COUNT(*) FROM columns c WHERE c.table_id = t.id) as col_count,
		       (SELECT COUNT(*) FROM foreign_keys fk WHERE fk.table_id = t.id) as fk_count
		FROM tables t
		ORDER BY t.name
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if rows.Scan(&t.ID, &t.Schema, &t.Name, &t.RowEstimate, &t.ColCount, &t.FKCount) == nil {
			tables = append(tables, t)
		}
	}
	jsonResp(w, tables)
}

type columnDetail struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Nullable    bool           `json:"nullable"`
	IsPK        bool           `json:"is_pk"`
	FKTarget    string         `json:"fk_target,omitempty"`
	Distinct    int            `json:"distinct"`
	IsState     bool           `json:"is_state"`
	IsCategoric bool           `json:"is_categoric"`
	Values      []valueInfo    `json:"values,omitempty"`
	JSONBPaths  []jsonbPathInfo `json:"jsonb_paths,omitempty"`
}

type valueInfo struct {
	Value     string `json:"value"`
	Frequency int    `json:"frequency"`
}

type jsonbPathInfo struct {
	Path         string `json:"path"`
	InferredType string `json:"inferred_type"`
	SampleValues string `json:"sample_values,omitempty"`
}

type tableDetail struct {
	tableInfo
	PrimaryKey  []string       `json:"primary_key"`
	ForeignKeys []fkInfo       `json:"foreign_keys"`
	Columns     []columnDetail `json:"columns"`
}

type fkInfo struct {
	SrcColumns string `json:"src_columns"`
	RefTable   string `json:"ref_table"`
	DstColumns string `json:"dst_columns"`
}

func (a *API) handleTableDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/tables/")
	if name == "" {
		http.Error(w, "table name required", 400)
		return
	}

	var td tableDetail
	err := a.store.DB().QueryRow(`
		SELECT t.id, t.schema, t.name, t.row_estimate,
		       (SELECT COUNT(*) FROM columns c WHERE c.table_id = t.id) as col_count,
		       (SELECT COUNT(*) FROM foreign_keys fk WHERE fk.table_id = t.id) as fk_count
		FROM tables t WHERE t.name = ?
	`, name).Scan(&td.ID, &td.Schema, &td.Name, &td.RowEstimate, &td.ColCount, &td.FKCount)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", 404)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Primary keys
	pkRows, _ := a.store.DB().Query(`SELECT column_name FROM primary_keys WHERE table_id = ?`, td.ID)
	if pkRows != nil {
		for pkRows.Next() {
			var col string
			if pkRows.Scan(&col) == nil {
				td.PrimaryKey = append(td.PrimaryKey, col)
			}
		}
		pkRows.Close()
	}

	// Foreign keys
	fkRows, _ := a.store.DB().Query(`
		SELECT fk.src_columns, rt.name, fk.dst_columns
		FROM foreign_keys fk
		JOIN tables rt ON rt.id = fk.ref_table_id
		WHERE fk.table_id = ?
	`, td.ID)
	if fkRows != nil {
		for fkRows.Next() {
			var fk fkInfo
			if fkRows.Scan(&fk.SrcColumns, &fk.RefTable, &fk.DstColumns) == nil {
				td.ForeignKeys = append(td.ForeignKeys, fk)
			}
		}
		fkRows.Close()
	}

	// Columns with stats
	colRows, _ := a.store.DB().Query(`
		SELECT c.name, c.type, c.nullable,
		       CASE WHEN pk.column_name IS NOT NULL THEN 1 ELSE 0 END,
		       COALESCE(fs.distinct_count, 0),
		       COALESCE(fs.is_state_like, 0),
		       COALESCE(fs.is_categorical, 0)
		FROM columns c
		LEFT JOIN primary_keys pk ON pk.table_id = c.table_id AND pk.column_name = c.name
		LEFT JOIN field_stats fs ON fs.column_id = c.id
		WHERE c.table_id = ?
		ORDER BY c.position
	`, td.ID)
	if colRows != nil {
		for colRows.Next() {
			var cd columnDetail
			var isPK, isState, isCat int
			if colRows.Scan(&cd.Name, &cd.Type, &cd.Nullable, &isPK, &cd.Distinct, &isState, &isCat) == nil {
				cd.IsPK = isPK == 1
				cd.IsState = isState == 1
				cd.IsCategoric = isCat == 1
				td.Columns = append(td.Columns, cd)
			}
		}
		colRows.Close()
	}

	// FK targets per column
	fkByCol := make(map[string]string)
	for _, fk := range td.ForeignKeys {
		for _, col := range strings.Split(fk.SrcColumns, ",") {
			dst := fk.RefTable
			if fk.DstColumns != "id" {
				dst += "." + fk.DstColumns
			}
			fkByCol[col] = dst
		}
	}
	for i := range td.Columns {
		if target, ok := fkByCol[td.Columns[i].Name]; ok {
			td.Columns[i].FKTarget = target
		}
	}

	// Values for state/categorical columns
	for i := range td.Columns {
		if td.Columns[i].IsState || td.Columns[i].IsCategoric {
			valRows, _ := a.store.DB().Query(`
				SELECT value, frequency FROM field_values fv
				JOIN columns c ON c.id = fv.column_id
				WHERE c.table_id = ? AND c.name = ?
				ORDER BY frequency DESC
			`, td.ID, td.Columns[i].Name)
			if valRows != nil {
				for valRows.Next() {
					var v valueInfo
					if valRows.Scan(&v.Value, &v.Frequency) == nil {
						td.Columns[i].Values = append(td.Columns[i].Values, v)
					}
				}
				valRows.Close()
			}
		}
	}

	// JSONB paths
	for i := range td.Columns {
		if td.Columns[i].Type == "jsonb" || td.Columns[i].Type == "json" {
			jRows, _ := a.store.DB().Query(`
				SELECT jp.path, jp.inferred_type, jp.sample_values
				FROM jsonb_paths jp
				JOIN columns c ON c.id = jp.column_id
				WHERE c.table_id = ? AND c.name = ?
				ORDER BY jp.path
			`, td.ID, td.Columns[i].Name)
			if jRows != nil {
				for jRows.Next() {
					var jp jsonbPathInfo
					if jRows.Scan(&jp.Path, &jp.InferredType, &jp.SampleValues) == nil {
						td.Columns[i].JSONBPaths = append(td.Columns[i].JSONBPaths, jp)
					}
				}
				jRows.Close()
			}
		}
	}

	jsonResp(w, td)
}

type queryResult struct {
	Query  string              `json:"query"`
	Tables []search.TableContext `json:"tables"`
}

func (a *API) handleQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "q parameter required", 400)
		return
	}

	result, err := search.Query(a.store, q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	jsonResp(w, queryResult{Query: result.Query, Tables: result.Tables})
}
