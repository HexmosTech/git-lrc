package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/terminology"
)

var terminologyCmd = &cobra.Command{
	Use:   "terminology",
	Short: "Manage the optional, user-controlled terminology dictionary",
	Long: "Terminology maps domain vocabulary (abbreviations, acronyms, business jargon) to exact dbctx " +
		"schema objects, as an additional lexical retrieval signal. It is completely independent from the " +
		"deterministic lexical index and the optional semantic embedding index, and is never populated " +
		"automatically — dbctx only generates a prompt for you to run through an LLM of your choice, and " +
		"only stores mappings you explicitly import.",
}

var terminologyPromptCmd = &cobra.Command{
	Use:   "prompt [dtx]",
	Short: "Print a self-contained prompt for generating a terminology dictionary",
	Long: "Prints, to stdout only, a prompt containing the full dbctx schema plus instructions for a large " +
		"external LLM (Claude, GPT, Gemini, ...) to interactively derive a terminology dictionary. dbctx never " +
		"calls an LLM itself. Paste the output into your preferred model, work through any clarifying " +
		"questions it asks, and save its final JSON output to import with 'dbctx terminology import'.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		prompt, err := terminology.GeneratePrompt(store)
		if err != nil {
			return fmt.Errorf("generate prompt: %w", err)
		}
		fmt.Print(prompt)
		return nil
	},
}

var terminologyImportCmd = &cobra.Command{
	Use:   "import [dtx] [terminology.json]",
	Short: "Validate and load a terminology dictionary into a .dtx file",
	Long: "Loads a JSON terminology dictionary (typically produced by working through " +
		"'dbctx terminology prompt' with an LLM) into a .dtx file. Every alias/target mapping is validated " +
		"against the actual schema — entries that don't resolve to a real table, column, or JSONB path are " +
		"rejected individually rather than failing the whole import.",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]
		jsonPath := args[1]

		data, err := os.ReadFile(jsonPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", jsonPath, err)
		}

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		result, err := terminology.Import(store, data)
		if err != nil {
			return fmt.Errorf("import terminology: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Accepted: %d\n", result.Accepted)
		if len(result.Rejected) > 0 {
			fmt.Fprintf(os.Stderr, "Rejected: %d\n", len(result.Rejected))
			for _, r := range result.Rejected {
				fmt.Fprintf(os.Stderr, "  term=%q alias=%q target=%q: %s\n", r.Term, r.Alias, r.Target, r.Reason)
			}
		}
		return nil
	},
}

var terminologyListCmd = &cobra.Command{
	Use:   "list [dtx]",
	Short: "List currently-imported terminology entries",
	Long:  "Prints every terminology mapping currently persisted in a .dtx file, for inspection.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		entries, err := terminology.List(store)
		if err != nil {
			return fmt.Errorf("list terminology: %w", err)
		}
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "No terminology imported.")
			return nil
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(entries)
		}

		for _, e := range entries {
			fmt.Printf("%-20s %-30q -> %s\n", e.Term, e.Alias, e.Target())
		}
		return nil
	},
}

func init() {
	terminologyListCmd.Flags().Bool("json", false, "Output as JSON")
	terminologyCmd.AddCommand(terminologyPromptCmd, terminologyImportCmd, terminologyListCmd)
	rootCmd.AddCommand(terminologyCmd)
}
