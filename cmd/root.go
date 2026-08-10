package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dbctx",
	Short: "Compile a PostgreSQL database into compact, queryable context",
	Long:  "dbctx builds a .dtx (SQLite) database context index from PostgreSQL: tables, relationships, field semantics, representative values, JSONB structure, and query-relevant metadata.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
