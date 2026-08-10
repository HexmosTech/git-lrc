package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/shrsv/dbctx/internal/db"
)

type Table struct {
	Schema      string
	Name        string
	OID         string
	RowEstimate float64
}

type Column struct {
	TableOID string
	Name     string
	DataType string
	Nullable bool
	Attnum   int
}

type Constraint struct {
	Kind       string // PK, UQ, FK
	TableOID   string
	ConName    string
	RefSchema  string
	RefTable   string
	SrcColumns string
	DstColumns string
}

type ExtractedSchema struct {
	Tables      []Table
	Columns     map[string][]Column // tableOID -> columns
	Constraints []Constraint
}

const extractionSQL = `
WITH tables AS (
    SELECT c.oid, n.nspname AS schema_name, c.relname AS table_name, c.reltuples AS row_estimate
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relkind IN ('r', 'p')
      AND n.nspname = ANY(string_to_array(
          COALESCE(NULLIF(current_setting('app.dbctx_schemas', true), ''), 'public'),
          ','
      ))
),

columns AS (
    SELECT
        t.oid AS table_oid,
        a.attnum,
        a.attname,
        format_type(a.atttypid, a.atttypmod) AS data_type,
        NOT a.attnotnull AS nullable
    FROM tables t
    JOIN pg_attribute a ON a.attrelid = t.oid
    WHERE a.attnum > 0
      AND NOT a.attisdropped
),

constraints AS (
    SELECT
        con.oid,
        con.conrelid AS table_oid,
        con.contype,
        con.conname,
        con.confrelid AS ref_table_oid,
        con.conkey,
        con.confkey
    FROM pg_constraint con
    JOIN tables t ON t.oid = con.conrelid
    WHERE con.contype IN ('p', 'u', 'f')
),

constraint_columns AS (
    SELECT
        con.oid AS constraint_oid,
        con.table_oid,
        con.contype,
        con.conname,
        con.ref_table_oid,
        src.attnum AS src_attnum,
        src.ord AS ord,
        dst.attnum AS dst_attnum
    FROM constraints con
    CROSS JOIN LATERAL
        unnest(con.conkey) WITH ORDINALITY AS src(attnum, ord)
    LEFT JOIN LATERAL
        unnest(con.confkey) WITH ORDINALITY AS dst(attnum, ord)
        ON dst.ord = src.ord
),

constraint_data AS (
    SELECT
        cc.constraint_oid,
        cc.table_oid,
        cc.contype,
        cc.conname,
        cc.ref_table_oid,
        string_agg(src.attname, ',' ORDER BY cc.ord) AS src_columns,
        string_agg(dst.attname, ',' ORDER BY cc.ord) AS dst_columns
    FROM constraint_columns cc
    JOIN pg_attribute src
      ON src.attrelid = cc.table_oid AND src.attnum = cc.src_attnum
    LEFT JOIN pg_attribute dst
      ON dst.attrelid = cc.ref_table_oid AND dst.attnum = cc.dst_attnum
    GROUP BY cc.constraint_oid, cc.table_oid, cc.contype, cc.conname, cc.ref_table_oid
)

SELECT
    'TABLE' AS kind,
    t.oid::text,
    t.schema_name,
    t.table_name,
    COALESCE(t.row_estimate::text, ''),
    c.attnum::text,
    c.attname,
    c.data_type,
    c.nullable::text,
    '',
    '',
    '',
    '',
    ''
FROM tables t
JOIN columns c ON c.table_oid = t.oid

UNION ALL

SELECT
    CASE cd.contype WHEN 'p' THEN 'PK' WHEN 'u' THEN 'UQ' WHEN 'f' THEN 'FK' END,
    t.oid::text,
    t.schema_name,
    t.table_name,
    '',
    '',
    '',
    '',
    '',
    cd.conname,
    COALESCE(rt.schema_name, ''),
    COALESCE(rt.table_name, ''),
    cd.src_columns,
    COALESCE(cd.dst_columns, '')
FROM constraint_data cd
JOIN tables t ON t.oid = cd.table_oid
LEFT JOIN tables rt ON rt.oid = cd.ref_table_oid

ORDER BY 3, 4, 1, 6;
`

func Extract(ctx context.Context, pg *db.PG, schemas string) (*ExtractedSchema, error) {
	_, err := pg.Conn.Exec(ctx, fmt.Sprintf("SET app.dbctx_schemas = '%s'", strings.ReplaceAll(schemas, "'", "''")))
	if err != nil {
		return nil, fmt.Errorf("set schemas: %w", err)
	}

	rows, err := pg.Conn.Query(ctx, extractionSQL)
	if err != nil {
		return nil, fmt.Errorf("query extraction: %w", err)
	}
	defer rows.Close()

	result := &ExtractedSchema{
		Columns: make(map[string][]Column),
	}

	tableByOID := make(map[string]*Table)

	for rows.Next() {
		var kind, tableOID, schemaName, tableName, rowEstimate string
		var attnum, attname, dataType, nullable string
		var conName, refSchema, refTable, srcCols, dstCols string

		err := rows.Scan(
			&kind, &tableOID, &schemaName, &tableName, &rowEstimate,
			&attnum, &attname, &dataType, &nullable,
			&conName, &refSchema, &refTable, &srcCols, &dstCols,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if _, exists := tableByOID[tableOID]; !exists {
			t := Table{Schema: schemaName, Name: tableName, OID: tableOID, RowEstimate: atof(rowEstimate)}
			tableByOID[tableOID] = &t
			result.Tables = append(result.Tables, t)
		}

		switch kind {
		case "TABLE":
			col := Column{
				TableOID: tableOID,
				Name:     attname,
				DataType: dataType,
				Nullable: nullable == "true",
				Attnum:   atoi(attnum),
			}
			result.Columns[tableOID] = append(result.Columns[tableOID], col)
		case "PK", "UQ", "FK":
			result.Constraints = append(result.Constraints, Constraint{
				Kind:       kind,
				TableOID:   tableOID,
				ConName:    conName,
				RefSchema:  refSchema,
				RefTable:   refTable,
				SrcColumns: srcCols,
				DstColumns: dstCols,
			})
		}
	}

	return result, rows.Err()
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func atof(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func QuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
