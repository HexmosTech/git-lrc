package reviewquery

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// RunQuery is the default action for `lrc query`: runs a saved alias or raw SQL
// and prints a table or JSON. With no argument it shows help (so users discover
// the schema and examples rather than silently running a default query).
func RunQuery(c *cli.Context) error {
	// Seed from flags placed BEFORE the positional arg (cli parses those).
	jsonOut := c.Bool("json")
	filter := Filter{From: c.String("from"), To: c.String("to"), Range: c.String("range")}

	// urfave/cli stops parsing flags at the first positional arg, so also scan
	// the remaining args for trailing flags (e.g. `lrc query stats --from 2024-01-01`).
	positionals, err := parseTrailingFlags(c.Args().Slice(), &jsonOut, &filter)
	if err != nil {
		return err
	}

	// No alias/SQL given -> show help instead of defaulting to a query.
	if len(positionals) == 0 || strings.TrimSpace(positionals[0]) == "" {
		return cli.ShowSubcommandHelp(c)
	}

	arg := strings.TrimSpace(positionals[0])
	sqlText, found, err := ResolveAlias(arg)
	if err != nil {
		return err
	}
	if !found {
		// Not a known alias — treat the positional args as raw SQL.
		sqlText = strings.Join(positionals, " ")
	}

	res, err := Run(filter, sqlText)
	if err != nil {
		return fmt.Errorf("%w\n\nRun a saved alias or valid SQL, e.g.:\n  lrc query stats\n  lrc query \"SELECT * FROM review_log LIMIT 5\"\nSee 'lrc query --help' for the table schema and 'lrc query list' for aliases", err)
	}

	if jsonOut {
		out, err := FormatJSON(res)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}
	fmt.Print(FormatTable(res))
	return nil
}

// RunQueryAdd saves a user alias: `lrc query add <name> "<sql>"`.
func RunQueryAdd(c *cli.Context) error {
	name := strings.TrimSpace(c.Args().Get(0))
	sqlText := strings.TrimSpace(c.Args().Get(1))
	if name == "" || sqlText == "" {
		return fmt.Errorf("usage: lrc query add <name> \"<sql>\"")
	}
	if err := AddAlias(name, sqlText); err != nil {
		return err
	}
	fmt.Printf("Saved alias %q.\n", name)
	return nil
}

// truncateSQL shortens a query for compact listing.
func truncateSQL(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace/newlines
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

// parseTrailingFlags pulls flags out of args that cli left unparsed (anything
// after the first positional). Supports `--flag value` and `--flag=value`.
// Returns the remaining positional args; sets jsonOut/filter via pointers.
// Returns an error if a bound flag (--from/--to/--range) is the last arg with
// no value following it, rather than silently swallowing the flag name into
// the positionals (where it would end up mangling the SQL/alias lookup).
func parseTrailingFlags(args []string, jsonOut *bool, filter *Filter) ([]string, error) {
	boundFlags := []struct {
		name string
		dest *string
	}{
		{"--from", &filter.From},
		{"--to", &filter.To},
		{"--range", &filter.Range},
	}

	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" || a == "-j" {
			*jsonOut = true
			continue
		}

		consumed := false
		for _, bf := range boundFlags {
			if val, ok := strings.CutPrefix(a, bf.name+"="); ok {
				*bf.dest = val
				consumed = true
				break
			}
			if a == bf.name {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("%s requires a value", bf.name)
				}
				*bf.dest = args[i+1]
				i++
				consumed = true
				break
			}
		}
		if consumed {
			continue
		}
		positionals = append(positionals, a)
	}
	return positionals, nil
}

// RunQueryList prints every alias and its source.
func RunQueryList(c *cli.Context) error {
	aliases, err := ListAliases()
	if err != nil {
		return err
	}
	if len(aliases) == 0 {
		fmt.Println("(no aliases)")
		return nil
	}
	for _, a := range aliases {
		fmt.Printf("%-16s %-10s %s\n", a.Name, "["+a.Source+"]", truncateSQL(a.SQL, 60))
	}
	return nil
}

// RunQueryView prints the SQL behind a named alias.
func RunQueryView(c *cli.Context) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return fmt.Errorf("usage: lrc query view <name>")
	}
	sqlText, found, err := ResolveAlias(name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no alias named %q", name)
	}
	fmt.Println(sqlText)
	return nil
}

// RunQueryDelete removes a user-defined alias.
func RunQueryDelete(c *cli.Context) error {
	name := strings.TrimSpace(c.Args().First())
	if name == "" {
		return fmt.Errorf("usage: lrc query delete <name>")
	}
	if err := DeleteAlias(name); err != nil {
		return err
	}
	fmt.Printf("Deleted alias %q.\n", name)
	return nil
}
