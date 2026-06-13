package britannica_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/britannica-cli/britannica"
)

// braveHTML returns a minimal Brave Search HTML page that contains two
// Britannica article snippets — enough for the parser to exercise both
// the happy path and URL deduplication.
func braveHTML(articles []struct{ URL, Title, Snippet string }) string {
	var sb strings.Builder
	_, _ = fmt.Fprint(&sb, `<!DOCTYPE html><html><body>`)
	for _, a := range articles {
		_, _ = fmt.Fprintf(&sb, `
<div class="snippet">
  <a href="%s" class="svelte-14r20fy l1">
    <div class="title search-snippet-title line-clamp-1 svelte-14r20fy" title="%s">%s</div>
  </a>
  <div class="generic-snippet">
    <div class="content desktop-default-regular t-primary svelte-1cwdgg3">%s</div>
  </div>
</div>`, a.URL, a.Title, a.Title, a.Snippet)
	}
	_, _ = fmt.Fprint(&sb, `</body></html>`)
	return sb.String()
}

func TestSearch(t *testing.T) {
	articles := []struct{ URL, Title, Snippet string }{
		{
			URL:     "https://www.britannica.com/science/quantum-mechanics-physics",
			Title:   "Quantum mechanics | Definition, Development, &amp; Equations | Britannica",
			Snippet: "The study of matter and light on the atomic scale.",
		},
		{
			URL:     "https://www.britannica.com/biography/Max-Planck",
			Title:   "Max Planck | Biography, Discoveries, &amp; Quantum Theory | Britannica",
			Snippet: "German physicist who originated quantum theory.",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the query is forwarded
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "site:britannica.com") {
			t.Errorf("query %q missing site:britannica.com", q)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, braveHTML(articles))
	}))
	defer srv.Close()

	cfg := britannica.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0

	c := britannica.NewClient(cfg)
	got, err := c.Search(context.Background(), "quantum", 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d articles, want 2", len(got))
	}

	if !strings.Contains(got[0].Title, "Quantum mechanics") {
		t.Errorf("first article title = %q, expected to contain 'Quantum mechanics'", got[0].Title)
	}
	if got[0].URL != articles[0].URL {
		t.Errorf("first URL = %q, want %q", got[0].URL, articles[0].URL)
	}
	if got[0].Category != "science" {
		t.Errorf("first category = %q, want %q", got[0].Category, "science")
	}
	if !strings.Contains(got[1].Title, "Max Planck") {
		t.Errorf("second article title = %q, expected to contain 'Max Planck'", got[1].Title)
	}
}

func TestSearchLimit(t *testing.T) {
	articles := make([]struct{ URL, Title, Snippet string }, 5)
	for i := range articles {
		articles[i] = struct{ URL, Title, Snippet string }{
			URL:   fmt.Sprintf("https://www.britannica.com/science/article-%d", i),
			Title: fmt.Sprintf("Article %d | Britannica", i),
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, braveHTML(articles))
	}))
	defer srv.Close()

	cfg := britannica.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0

	c := britannica.NewClient(cfg)
	got, err := c.Search(context.Background(), "physics", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %d results with limit 3, want 3", len(got))
	}
}

func TestSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := britannica.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0

	c := britannica.NewClient(cfg)
	_, err := c.Search(context.Background(), "physics", 5)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
