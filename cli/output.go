package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tamnd/britannica-cli/britannica"
)

// outputArticles writes articles to w in the requested format (text or json).
func outputArticles(w io.Writer, arts []britannica.Article, format string) error {
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(arts); err != nil {
			return err
		}
	case "jsonl":
		enc := json.NewEncoder(w)
		for _, a := range arts {
			if err := enc.Encode(a); err != nil {
				return err
			}
		}
	default: // text
		for _, a := range arts {
			_, _ = fmt.Fprintf(w, "%s\n", a.Title)
			if a.Category != "" {
				_, _ = fmt.Fprintf(w, "  Category: %s\n", a.Category)
			}
			_, _ = fmt.Fprintf(w, "  URL: %s\n", a.URL)
			if a.Summary != "" {
				sum := a.Summary
				if len(sum) > 200 {
					sum = sum[:200] + "..."
				}
				_, _ = fmt.Fprintf(w, "  %s\n", sum)
			}
			_, _ = fmt.Fprintf(w, "\n")
		}
	}
	return nil
}
