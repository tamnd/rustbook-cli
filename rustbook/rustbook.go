// Package rustbook is the library behind the rustbook command line:
// the HTTP client, request shaping, and the typed data models for the
// Rust Programming Language book at doc.rust-lang.org.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package rustbook

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to the server. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "rustbook/dev (+https://github.com/tamnd/rustbook-cli)"

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "doc.rust-lang.org"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Client talks to the Rust book site over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 200ms
// minimum gap between requests, and five retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
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
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	return min(time.Duration(attempt)*500*time.Millisecond, 5*time.Second)
}

// Chapter is one entry from the Rust book table of contents. Rank is the
// 1-based position in the full flat list; Number is the dotted section number
// like "1", "1.1", or "" for front-matter entries; Title is plain text.
type Chapter struct {
	Rank   int    `json:"rank"   csv:"rank"   tsv:"rank"`
	Number string `json:"number" csv:"number" tsv:"number"`
	Title  string `json:"title"  csv:"title"  tsv:"title"`
	URL    string `json:"url"    csv:"url"    tsv:"url"`
}

var (
	linkRe   = regexp.MustCompile(`(?s)<a href="([\w.-]+)" target="_parent">(.*?)</a>`)
	strongRe = regexp.MustCompile(`<strong[^>]*>([^<]+)</strong>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
)

// Chapters fetches the Rust book table of contents and returns all chapters
// and sections as a flat list in document order.
func (c *Client) Chapters(ctx context.Context) ([]*Chapter, error) {
	body, err := c.Get(ctx, BaseURL+"/book/toc.html")
	if err != nil {
		return nil, err
	}
	html := string(body)
	matches := linkRe.FindAllStringSubmatch(html, -1)
	var chapters []*Chapter
	rank := 1
	for _, m := range matches {
		href := m[1]
		inner := m[2]
		// Extract number from <strong>
		numMatch := strongRe.FindStringSubmatch(inner)
		number := ""
		if numMatch != nil {
			number = strings.TrimRight(numMatch[1], ".")
		}
		// Extract title: remove the <strong>...</strong> block first, then strip remaining tags
		cleaned := strongRe.ReplaceAllString(inner, "")
		title := strings.TrimSpace(tagRe.ReplaceAllString(cleaned, ""))
		// Build full URL
		url := BaseURL + "/book/" + href
		chapters = append(chapters, &Chapter{
			Rank:   rank,
			Number: number,
			Title:  title,
			URL:    url,
		})
		rank++
	}
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters found")
	}
	return chapters, nil
}
