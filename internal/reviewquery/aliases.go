package reviewquery

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/HexmosTech/git-lrc/configpath"
	"github.com/HexmosTech/git-lrc/storage"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// builtinAliases ship with the binary so `lrc query <name>` works even before
// the installer writes ~/.lrc/queries.toml. User-defined aliases override these.
func builtinAliases() map[string]string {
	return map[string]string{
		"stats":     "SELECT action AS Action, COUNT(*) AS Commits, ROUND(AVG(iterations),1) AS AvgIter, ROUND(AVG(coverage),1) AS AvgCoveragePct FROM review_log GROUP BY action ORDER BY Commits DESC",
		"by-author": "SELECT author AS Author, COUNT(*) AS Commits, SUM(action = 'reviewed') AS Reviewed FROM review_log GROUP BY author ORDER BY Commits DESC",
		"recent":    "SELECT short_hash AS Hash, date AS Date, action AS Action, subject AS Subject FROM review_log ORDER BY date DESC LIMIT 20",
	}
}

// AliasInfo describes one alias and where it came from.
type AliasInfo struct {
	Name   string
	SQL    string
	Source string // "built-in" or "user"
}

// queriesPath returns ~/.lrc/queries.toml.
func queriesPath() (string, error) {
	dir, err := configpath.ResolveLRCDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "queries.toml"), nil
}

// loadUserAliases reads ~/.lrc/queries.toml ([queries] table). Missing file is
// not an error — it returns an empty map.
func loadUserAliases() (map[string]string, error) {
	path, err := queriesPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("failed to access user aliases file %s: %w", path, err)
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to parse user aliases file %s: %w", path, err)
	}
	// A non-empty file that lacks the [queries] table entirely is malformed —
	// surface that instead of silently loading zero aliases.
	if len(k.Keys()) > 0 && !k.Exists("queries") {
		return nil, fmt.Errorf("user aliases file %s has no [queries] table", path)
	}
	out := map[string]string{}
	maps.Copy(out, k.StringMap("queries"))
	return out, nil
}

// ResolveAlias returns the SQL for an alias name (user file wins over built-in).
func ResolveAlias(name string) (string, bool, error) {
	user, err := loadUserAliases()
	if err != nil {
		return "", false, err
	}
	if sql, ok := user[name]; ok {
		return sql, true, nil
	}
	if sql, ok := builtinAliases()[name]; ok {
		return sql, true, nil
	}
	return "", false, nil
}

// ListAliases returns every alias (built-in + user) sorted by name; a user
// alias shadows a built-in of the same name.
func ListAliases() ([]AliasInfo, error) {
	user, err := loadUserAliases()
	if err != nil {
		return nil, err
	}
	merged := map[string]AliasInfo{}
	for name, sql := range builtinAliases() {
		merged[name] = AliasInfo{Name: name, SQL: sql, Source: "built-in"}
	}
	for name, sql := range user {
		merged[name] = AliasInfo{Name: name, SQL: sql, Source: "user"}
	}
	names := make([]string, 0, len(merged))
	for n := range merged {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]AliasInfo, 0, len(names))
	for _, n := range names {
		out = append(out, merged[n])
	}
	return out, nil
}

// AddAlias saves (or overwrites) a user alias in ~/.lrc/queries.toml.
func AddAlias(name, sql string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("alias name cannot be empty")
	}
	if strings.ContainsAny(name, ". \t") {
		return fmt.Errorf("alias name %q may not contain spaces or dots", name)
	}
	if strings.TrimSpace(sql) == "" {
		return fmt.Errorf("alias SQL cannot be empty")
	}
	if err := validateReadOnlySQL(sql); err != nil {
		return fmt.Errorf("alias SQL rejected: %w", err)
	}
	user, err := loadUserAliases()
	if err != nil {
		return err
	}
	user[name] = sql
	return writeUserAliases(user)
}

// DeleteAlias removes a user alias. Built-in aliases cannot be deleted.
func DeleteAlias(name string) error {
	user, err := loadUserAliases()
	if err != nil {
		return err
	}
	if _, ok := user[name]; !ok {
		if _, isBuiltin := builtinAliases()[name]; isBuiltin {
			return fmt.Errorf("%q is a built-in alias and cannot be deleted", name)
		}
		return fmt.Errorf("no user alias named %q", name)
	}
	delete(user, name)
	return writeUserAliases(user)
}

// writeUserAliases serializes the alias map to ~/.lrc/queries.toml atomically.
func writeUserAliases(aliases map[string]string) error {
	path, err := queriesPath()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(aliases))
	for n := range aliases {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# git-lrc saved queries. Managed by `lrc query --add/--delete`.\n")
	b.WriteString("[queries]\n")
	for _, n := range names {
		b.WriteString(n)
		b.WriteString(" = ")
		b.WriteString(strconv.Quote(aliases[n]))
		b.WriteString("\n")
	}
	return storage.WriteFileAtomically(path, []byte(b.String()), 0o644)
}
