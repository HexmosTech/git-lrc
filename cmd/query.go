package cmd

import (
	"fmt"
	"os"

	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/report"
	"github.com/shrsv/dbctx/internal/search"
	"github.com/shrsv/dbctx/internal/semantic"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query [dtx] [query text...]",
	Short: "Query the .dtx index with natural language",
	Long:  "Searches tables, columns, and values using fuzzy matching and FTS, expands via foreign keys, and outputs relevant schema context. If the .dtx has a semantic index (see 'dbctx build'), also folds in embedding-based similarity as an additional ranking signal.",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]
		queryText := joinStrings(args[1:], " ")
		noSemantic, _ := cmd.Flags().GetBool("no-semantic")

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		var scorer search.SemanticScorer
		if !noSemantic {
			scorer, err = semantic.OpenScorer(store, progressLogger(os.Stderr))
			if err != nil {
				fmt.Fprintf(os.Stderr, "dbctx: semantic search unavailable (%v); using lexical search only\n", err)
				scorer = nil
			}
		}

		result, err := search.QueryHybrid(store, queryText, scorer)
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}

		report.FormatQueryResult(os.Stdout, result)
		return nil
	},
}

func init() {
	queryCmd.Flags().Bool("no-semantic", false, "Skip the semantic retrieval signal even if the .dtx has one (lexical/fuzzy search only)")
	rootCmd.AddCommand(queryCmd)
}
