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
	Long:  "Searches tables, columns, and values using fuzzy matching and FTS, expands via foreign keys, and outputs relevant schema context. If the .dtx has a semantic index (see 'dbctx build'), also folds in embedding-based similarity as an additional ranking signal.\n\nBy default this includes FK-expanded tables (not just direct hits) — the join context an LLM needs to actually answer the query. Pass --matched-only to restrict output to tables that scored a direct match.",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]
		queryText := joinStrings(args[1:], " ")
		noSemantic, _ := cmd.Flags().GetBool("no-semantic")
		matchedOnly, _ := cmd.Flags().GetBool("matched-only")

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

		if matchedOnly {
			matched := result.Tables[:0]
			for _, t := range result.Tables {
				if t.IsMatch {
					matched = append(matched, t)
				}
			}
			result.Tables = matched
		}

		report.FormatQueryResult(os.Stdout, result)
		return nil
	},
}

func init() {
	queryCmd.Flags().Bool("no-semantic", false, "Skip the semantic retrieval signal even if the .dtx has one (lexical/fuzzy search only)")
	queryCmd.Flags().Bool("matched-only", false, "Restrict output to tables that scored a direct match, excluding FK-expanded context tables")
	rootCmd.AddCommand(queryCmd)
}
