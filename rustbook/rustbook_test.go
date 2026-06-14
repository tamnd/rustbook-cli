package rustbook

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

const fakeTOC = `<html><body>
<ol class="chapter">
<li class="chapter-item expanded "><span class="chapter-link-wrapper"><a href="title-page.html" target="_parent">The Rust Programming Language</a></span></li>
<li class="chapter-item expanded "><span class="chapter-link-wrapper"><a href="ch01-00-getting-started.html" target="_parent"><strong aria-hidden="true">1.</strong> Getting Started</a></span>
<ol class="section">
<li class="chapter-item expanded "><span class="chapter-link-wrapper"><a href="ch01-01-installation.html" target="_parent"><strong aria-hidden="true">1.1.</strong> Installation</a></span></li>
</ol>
</li>
</ol>
</body></html>`

func TestChapters(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, fakeTOC)
	}))
	defer ts.Close()

	c := NewClient()
	c.Rate = 0
	// Point the client at the test server by temporarily overriding the URL via
	// a round-trip through Get which uses the full URL, so we call Chapters via
	// a patched client that resolves to the test server.
	origGet := c.HTTP
	_ = origGet
	// We test Chapters by wrapping: replace the BaseURL by patching Get.
	// Since Chapters calls c.Get(ctx, BaseURL+"/book/toc.html"), we need to
	// intercept at the HTTP level. Use a custom transport.
	c.HTTP = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &prefixRewriter{
			from:      BaseURL,
			to:        ts.URL,
			transport: http.DefaultTransport,
		},
	}

	chapters, err := c.Chapters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("want 3 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "The Rust Programming Language" {
		t.Errorf("Title[0] = %q", chapters[0].Title)
	}
	if chapters[0].Number != "" {
		t.Errorf("Number[0] = %q, want empty", chapters[0].Number)
	}
	if chapters[1].Number != "1" {
		t.Errorf("Number[1] = %q, want 1", chapters[1].Number)
	}
	if chapters[1].Title != "Getting Started" {
		t.Errorf("Title[1] = %q, want Getting Started", chapters[1].Title)
	}
	if chapters[2].Number != "1.1" {
		t.Errorf("Number[2] = %q, want 1.1", chapters[2].Number)
	}
}

// prefixRewriter rewrites outbound request URLs from one prefix to another,
// so tests can hit a local httptest.Server using the real URL constants.
type prefixRewriter struct {
	from      string
	to        string
	transport http.RoundTripper
}

func (p *prefixRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	if len(url) >= len(p.from) && url[:len(p.from)] == p.from {
		url = p.to + url[len(p.from):]
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, url, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return p.transport.RoundTrip(newReq)
}
