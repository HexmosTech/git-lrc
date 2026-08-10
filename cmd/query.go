package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/report"
	"github.com/shrsv/dbctx/internal/search"
)

var queryCmd = &cobra.Command{
	Use:   "query [dtx] [query text...]",
	Short: "Query the .dtx index with natural language",
	Long:  "Searches tables, columns, and values using fuzzy matching and FTS, expands via foreign keys, and outputs relevant schema context.",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]
		queryText := joinStrings(args[1:], " ")

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		result, err := search.Query(store, queryText)
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}

		report.FormatQueryResult(os.Stdout, result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)
}
