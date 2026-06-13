package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/britannica-cli/britannica"
)

func newSearchCmd() *cobra.Command {
	var (
		limit  int
		output string
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search Britannica articles by keyword",
		Long: `Search Encyclopedia Britannica articles by keyword.

Results are sourced from Britannica's public article index and include
the article title, URL, category, and a short summary.

Examples:
  bri search "quantum physics"
  bri search gravity --limit 5
  bri search "black holes" -o json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			if query == "" {
				return wrapUsage(cmd.Usage())
			}

			cfg := britannica.DefaultConfig()
			c := britannica.NewClient(cfg)

			arts, err := c.Search(cmd.Context(), query, limit)
			if err != nil {
				return printErr(cmd.ErrOrStderr(), "%v", err)
			}

			if len(arts) == 0 {
				_, _ = cmd.ErrOrStderr().Write([]byte("no results found\n"))
				return nil
			}

			return outputArticles(cmd.OutOrStdout(), arts, output)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of results")
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text, json, jsonl")
	return cmd
}
