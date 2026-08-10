package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/report"
)

var reportCmd = &cobra.Command{
	Use:   "report [dtx]",
	Short: "Dump all intelligence from the .dtx index",
	Long:  "Outputs schema, state fields, categorical fields, JSONB structure, relationships, and stats from the .dtx file.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		return report.ReportAll(os.Stdout, store)
	},
}

func init() {
	rootCmd.AddCommand(reportCmd)
}
