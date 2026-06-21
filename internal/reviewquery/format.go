package reviewquery

import (
	"encoding/json"
	"strings"
)

// FormatTable renders a QueryResult as an aligned, human-readable table.
func FormatTable(r QueryResult) string {
	if len(r.Columns) == 0 {
		return "(no columns)\n"
	}

	widths := make([]int, len(r.Columns))
	for i, c := range r.Columns {
		widths[i] = len(c)
	}
	for _, row := range r.Rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i, c := range cells {
			b.WriteString(c)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(c)+2))
			}
		}
		b.WriteString("\n")
	}

	writeRow(r.Columns)
	sep := make([]string, len(r.Columns))
	for i := range sep {
		sep[i] = strings.Repeat("-", widths[i])
	}
	writeRow(sep)
	for _, row := range r.Rows {
		writeRow(row)
	}

	if len(r.Rows) == 0 {
		b.WriteString("(no rows)\n")
	}
	return b.String()
}

// FormatJSON renders a QueryResult as a JSON array of row objects, preserving
// column order. All values are strings (the engine stringifies cells).
//
// This builds the object syntax by hand rather than json.Marshal-ing a
// map[string]string per row: encoding/json has no way to preserve key order
// for a Go map (it always sorts map keys alphabetically), and column order is
// part of this format's contract. Each key/value is still run through
// json.Marshal so escaping stays correct.
func FormatJSON(r QueryResult) (string, error) {
	var b strings.Builder
	b.WriteString("[")
	for ri, row := range r.Rows {
		if ri > 0 {
			b.WriteString(",")
		}
		b.WriteString("{")
		for ci, col := range r.Columns {
			if ci > 0 {
				b.WriteString(",")
			}
			key, err := json.Marshal(col)
			if err != nil {
				return "", err
			}
			val := ""
			if ci < len(row) {
				val = row[ci]
			}
			valJSON, err := json.Marshal(val)
			if err != nil {
				return "", err
			}
			b.Write(key)
			b.WriteString(":")
			b.Write(valJSON)
		}
		b.WriteString("}")
	}
	b.WriteString("]")
	return b.String(), nil
}
