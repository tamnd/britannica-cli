// Package cli builds the bri command tree on top of the britannica library.
package cli

import (
	"github.com/spf13/cobra"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Root builds the root command and its subtree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "bri",
		Short: "Browse Encyclopedia Britannica articles",
		Long: `Browse Encyclopedia Britannica articles from the command line.

Search the world's leading encyclopedia by keyword and get back article
titles, URLs, categories, and summaries in text, JSON, or JSONL format.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(newSearchCmd())
	return root
}
