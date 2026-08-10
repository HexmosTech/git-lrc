#!/usr/bin/env python3

import os
import re
import subprocess
import sys
from collections import defaultdict


SQL = r"""
WITH tables AS (
    SELECT c.oid, n.nspname AS schema_name, c.relname AS table_name
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relkind IN ('r', 'p')
      AND n.nspname = ANY(string_to_array(
          COALESCE(NULLIF(current_setting('app.llm_schema_schemas', true), ''), 'public'),
          ','
      ))
),

columns AS (
    SELECT
        c.oid AS table_oid,
        a.attnum,
        a.attname,
        format_type(a.atttypid, a.atttypmod) AS data_type,
        NOT a.attnotnull AS nullable
    FROM tables c
    JOIN pg_attribute a ON a.attrelid = c.oid
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
        string_agg(
            src.attname,
            ',' ORDER BY cc.ord
        ) AS src_columns,
        string_agg(
            dst.attname,
            ',' ORDER BY cc.ord
        ) AS dst_columns
    FROM constraint_columns cc
    JOIN pg_attribute src
      ON src.attrelid = cc.table_oid
     AND src.attnum = cc.src_attnum
    LEFT JOIN pg_attribute dst
      ON dst.attrelid = cc.ref_table_oid
     AND dst.attnum = cc.dst_attnum
    GROUP BY
        cc.constraint_oid,
        cc.table_oid,
        cc.contype,
        cc.conname,
        cc.ref_table_oid
)

SELECT
    'TABLE' AS kind,
    t.oid::text AS table_oid,
    t.schema_name,
    t.table_name,
    c.attnum::text,
    c.attname,
    c.data_type,
    c.nullable::text,
    '' AS constraint_name,
    '' AS ref_schema,
    '' AS ref_table,
    '' AS src_columns,
    '' AS dst_columns
FROM tables t
JOIN columns c ON c.table_oid = t.oid

UNION ALL

SELECT
    CASE cd.contype
        WHEN 'p' THEN 'PK'
        WHEN 'u' THEN 'UQ'
        WHEN 'f' THEN 'FK'
    END AS kind,
    t.oid::text,
    t.schema_name,
    t.table_name,
    '' AS attnum,
    '' AS attname,
    '' AS data_type,
    '' AS nullable,
    cd.conname,
    COALESCE(rt.schema_name, ''),
    COALESCE(rt.table_name, ''),
    cd.src_columns,
    cd.dst_columns
FROM constraint_data cd
JOIN tables t ON t.oid = cd.table_oid
LEFT JOIN tables rt ON rt.oid = cd.ref_table_oid

ORDER BY schema_name, table_name, kind, attnum;
"""


LEGEND = (
    "# LiveReview schema (public). "
    "types: int2/int4/int8=smallint/integer/bigint, bool=boolean, "
    "vc=varchar, c=char (lengths omitted), "
    "timestamp=timestamp w/o tz, timestamptz=timestamp with tz, "
    "float8=double precision. "
    "flags: ^ PK; = unique; ? nullable (no flag -> NOT NULL); "
    ">table[.col] FK (col omitted -> id)"
)


def abbreviate_type(data_type):
    if data_type.startswith("timestamp without time zone"):
        data_type = data_type.replace("timestamp without time zone", "timestamp")
    elif data_type.startswith("timestamp with time zone"):
        data_type = data_type.replace("timestamp with time zone", "timestamptz")

    data_type = re.sub(r"^character varying(\(\d+\))?", "vc", data_type)
    data_type = re.sub(r"^character(\(\d+\))?", "c", data_type)
    data_type = data_type.replace("double precision", "float8")
    data_type = data_type.replace("smallint", "int2")
    data_type = data_type.replace("bigint", "int8")
    data_type = data_type.replace("integer", "int4")
    data_type = data_type.replace("boolean", "bool")

    return data_type


def psql(database_url, schemas):
    env = os.environ.copy()

    # Used by the SQL query to select schemas.
    env["PGOPTIONS"] = (
        env.get("PGOPTIONS", "")
        + f" -c app.llm_schema_schemas={schemas}"
    )

    result = subprocess.run(
        [
            "psql",
            database_url,
            "--no-psqlrc",
            "--tuples-only",
            "--no-align",
            "--field-separator", "\t",
            "--command", SQL,
        ],
        env=env,
        text=True,
        capture_output=True,
    )

    if result.returncode != 0:
        print(result.stderr, file=sys.stderr)
        sys.exit(result.returncode)

    return result.stdout


def generate(output):
    schemas = os.getenv("LLM_SCHEMA_SCHEMAS", "public")
    database_url = os.getenv("DATABASE_URL")

    if not database_url:
        print("DATABASE_URL is required", file=sys.stderr)
        sys.exit(1)

    rows = psql(database_url, schemas)

    tables = defaultdict(lambda: {
        "columns": [],
        "pk": [],
        "uq": [],
        "fk": [],
    })

    for line in rows.splitlines():
        if not line:
            continue

        fields = line.split("\t")
        if len(fields) != 13:
            continue

        (
            kind,
            table_oid,
            schema,
            table,
            attnum,
            column,
            data_type,
            nullable,
            constraint_name,
            ref_schema,
            ref_table,
            src_columns,
            dst_columns,
        ) = fields

        key = f"{schema}.{table}" if schema != "public" else table

        if kind == "TABLE":
            tables[key]["columns"].append({
                "name": column,
                "type": abbreviate_type(data_type),
                "nullable": nullable == "true",
            })

        elif kind == "PK":
            tables[key]["pk"].append(src_columns)

        elif kind == "UQ":
            tables[key]["uq"].append(src_columns)

        elif kind == "FK":
            tables[key]["fk"].append({
                "src": src_columns,
                "schema": ref_schema,
                "table": ref_table,
                "dst": dst_columns,
            })

    lines = [LEGEND, ""]

    for table in sorted(tables):
        data = tables[table]

        entries = []

        pk_columns = set()
        for pk in data["pk"]:
            if "," in pk:
                entries.append(f"^({pk})")
            else:
                pk_columns.add(pk)

        unique_columns = set()
        for uq in data["uq"]:
            if "," in uq:
                entries.append(f"=({uq})")
            else:
                unique_columns.add(uq)

        fk_by_column = {}

        for fk in data["fk"]:
            src = fk["src"]
            dst = fk["dst"]
            dst_suffix = "" if dst == "id" else f".{dst}"

            target = (
                f"{fk['schema']}.{fk['table']}{dst_suffix}"
                if fk["schema"] != "public"
                else f"{fk['table']}{dst_suffix}"
            )

            if "," in src:
                entries.append(f">({src})>{target}")
            else:
                fk_by_column[src] = target

        for col in data["columns"]:
            parts = [col["name"], col["type"]]

            if col["name"] in pk_columns:
                parts.append("^")

            if col["name"] in unique_columns:
                parts.append("=")

            if col["name"] in fk_by_column:
                parts.append(f">{fk_by_column[col['name']]}")

            if col["nullable"]:
                parts.append("?")

            entries.append(" ".join(parts))

        lines.append(f"{table}: " + ", ".join(entries))
        lines.append("")

    result = "\n".join(lines).rstrip() + "\n"

    if output:
        with open(output, "w") as f:
            f.write(result)
    else:
        print(result, end="")


if __name__ == "__main__":
    output = sys.argv[1] if len(sys.argv) > 1 else None
    generate(output)
