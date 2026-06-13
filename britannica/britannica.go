// Package britannica is the library behind the bri command line:
// the HTTP client, request shaping, and the typed data models for Britannica.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
//
// Britannica's public-facing site sits behind Cloudflare's managed challenge,
// which blocks plain HTTP clients. We route around this by querying Brave
// Search for site:britannica.com results and parsing the HTML response — the
// same data users would see, no credentials required.
package britannica

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUserAgent is a real browser UA that Brave Search accepts.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// DefaultBaseURL is the Brave Search endpoint we query for Britannica results.
const DefaultBaseURL = "https://search.brave.com"

// Config holds client settings.
type Config struct {
	// BaseURL is the search backend base URL. Override in tests.
	BaseURL string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate time.Duration
	// Retries is how many times to retry transient errors.
	Retries int
	// UserAgent to send with every request.
	UserAgent string
}

// DefaultConfig returns production defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   DefaultBaseURL,
		Rate:      300 * time.Millisecond,
		Retries:   3,
		UserAgent: DefaultUserAgent,
	}
}

// Article represents a single Britannica encyclopedia article.
type Article struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

// Client talks to the Brave Search HTML endpoint to discover Britannica articles.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client using the given Config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Search returns up to limit Britannica articles matching query.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Article, error) {
	u := c.cfg.BaseURL + "/search?q=" + url.QueryEscape("site:britannica.com "+query)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}
	arts := parseResults(string(body))
	if limit > 0 && len(arts) > limit {
		arts = arts[:limit]
	}
	return arts, nil
}

// get fetches url with retries and rate-limiting.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// parseResults extracts Britannica articles from Brave Search HTML.
// It scans for href="https://www.britannica.com/..." anchors and picks up the
// adjacent title div and snippet text using simple string scanning — no
// external HTML parser, no golang.org/x/net/html dependency.
func parseResults(htmlStr string) []Article {
	const prefix = `href="https://www.britannica.com/`
	var arts []Article
	seen := make(map[string]bool)

	i := 0
	for i < len(htmlStr) {
		idx := strings.Index(htmlStr[i:], prefix)
		if idx < 0 {
			break
		}
		idx += i

		// Extract URL
		urlStart := idx + 6 // skip href="
		urlEnd := strings.Index(htmlStr[urlStart:], `"`)
		if urlEnd < 0 {
			i = urlStart
			continue
		}
		articleURL := htmlStr[urlStart : urlStart+urlEnd]
		i = urlStart + urlEnd + 1

		// Skip kids.britannica, videos, summary pages
		if strings.Contains(articleURL, "kids.britannica") ||
			strings.Contains(articleURL, "/video/") ||
			strings.Contains(articleURL, "/summary/") ||
			seen[articleURL] {
			continue
		}
		seen[articleURL] = true

		// Search the next 3000 chars for title and snippet
		chunk := htmlStr[idx:]
		if len(chunk) > 3000 {
			chunk = chunk[:3000]
		}

		title := extractTitle(chunk)
		summary := extractSnippet(chunk)
		category := categoryFromURL(articleURL)

		if title == "" {
			continue
		}
		arts = append(arts, Article{
			Title:    html.UnescapeString(title),
			Summary:  html.UnescapeString(summary),
			URL:      articleURL,
			Category: category,
		})
	}
	return arts
}

// extractTitle finds the title div text in a chunk of Brave Search HTML.
// The pattern is: class="title ...[contains search-snippet-title]..." title="TITLE">TITLE</div>
func extractTitle(chunk string) string {
	const marker = `class="title `
	idx := strings.Index(chunk, marker)
	if idx < 0 {
		return ""
	}
	// Look for title="..." attribute which Brave sets
	rest := chunk[idx:]
	ti := strings.Index(rest, `title="`)
	if ti < 0 {
		// Fall back: find the text content between > and </div>
		gi := strings.Index(rest, ">")
		if gi < 0 {
			return ""
		}
		end := strings.Index(rest[gi+1:], "</div>")
		if end < 0 {
			return ""
		}
		return strings.TrimSpace(rest[gi+1 : gi+1+end])
	}
	start := ti + 7
	end := strings.Index(rest[start:], `"`)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[start : start+end])
}

// extractSnippet finds the snippet text in a chunk of Brave Search HTML.
// Brave wraps the description in a div with class containing "content desktop-default-regular".
func extractSnippet(chunk string) string {
	const marker = `content desktop-default-regular`
	idx := strings.Index(chunk, marker)
	if idx < 0 {
		return ""
	}
	rest := chunk[idx:]
	// Find the opening >
	gi := strings.Index(rest, ">")
	if gi < 0 {
		return ""
	}
	text := rest[gi+1:]
	// Skip HTML comments and date spans that Brave inserts
	text = stripHTMLComments(text)
	text = stripSpans(text)
	// Now strip remaining tags
	text = stripTags(text)
	text = strings.TrimSpace(text)
	// Truncate at the first </div> boundary
	if end := strings.Index(text, "</div>"); end >= 0 {
		text = text[:end]
	}
	// Remove extra whitespace
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}

// stripHTMLComments removes <!--...--> comments.
func stripHTMLComments(s string) string {
	for {
		start := strings.Index(s, "<!--")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "-->")
		if end < 0 {
			break
		}
		s = s[:start] + s[start+end+3:]
	}
	return s
}

// stripSpans removes <span ...>...</span> wrappers (date prefix in Brave snippets).
func stripSpans(s string) string {
	for {
		start := strings.Index(s, "<span")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "</span>")
		if end < 0 {
			break
		}
		s = s[:start] + s[start+end+7:]
	}
	return s
}

// stripTags removes all remaining HTML tags.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, ch := range s {
		switch {
		case ch == '<':
			inTag = true
		case ch == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// categoryFromURL infers a category from the Britannica URL path.
// e.g. /science/quantum-mechanics → "science"
func categoryFromURL(u string) string {
	// Strip scheme and host
	path := u
	if after, ok := strings.CutPrefix(path, "https://www.britannica.com"); ok {
		path = after
	}
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}
