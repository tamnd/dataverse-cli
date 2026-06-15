// Package dataverse is the library behind the dataverse command line:
// the HTTP client, request shaping, and the typed data models for the
// Harvard Dataverse repository.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package dataverse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "dataverse.harvard.edu"

// baseURL is the search endpoint every request is built from.
const baseURL = "https://dataverse.harvard.edu/api/search"

// DefaultUserAgent identifies the client to Dataverse. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "dataverse-cli/0.1.0 (+https://github.com/tamnd/dataverse-cli)"

// Config holds the tunable knobs for the HTTP client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns sensible defaults: a 500 ms pace, three retries, and a
// 30 s timeout.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to the Harvard Dataverse search API over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client built from DefaultConfig.
func NewClient() *Client {
	return NewClientWithConfig(DefaultConfig())
}

// NewClientWithConfig returns a Client built from cfg.
func NewClientWithConfig(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Get fetches rawURL and returns the response body. It paces and retries
// according to the client's settings. The caller owns nothing extra; the body
// is read fully and closed here.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff(attempt)):
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
	req.Header.Set("Accept", "application/json")

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

// backoff returns the delay before a given retry attempt.
func (c *Client) backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types — match the Dataverse search JSON shape exactly ---

type wireItem struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	URL         string          `json:"url"`
	GlobalID    string          `json:"global_id"`
	Description string          `json:"description"`
	PublishedAt string          `json:"published_at"`
	Publisher   string          `json:"publisher"`
	Subjects    []string        `json:"subjects"`
	FileCount   int             `json:"fileCount"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	Authors     json.RawMessage `json:"authors"`
}

type wireData struct {
	TotalCount int        `json:"total_count"`
	Start      int        `json:"start"`
	PerPage    int        `json:"per_page"`
	Items      []wireItem `json:"items"`
}

type wireResp struct {
	Status string   `json:"status"`
	Data   wireData `json:"data"`
}

// Dataset is the public record type: one dataset entry from Harvard Dataverse.
type Dataset struct {
	ID          string   `json:"id"                   kit:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url,omitempty"`
	Description string   `json:"description,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	FileCount   int      `json:"file_count,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

// datasetFromWire converts a wire item into the public Dataset type.
func datasetFromWire(item wireItem) *Dataset {
	return &Dataset{
		ID:          item.GlobalID,
		Title:       item.Name,
		URL:         item.URL,
		Description: item.Description,
		Publisher:   item.Publisher,
		Subjects:    item.Subjects,
		Authors:     parseAuthors(item.Authors),
		FileCount:   item.FileCount,
		PublishedAt: item.PublishedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

// parseAuthors handles the authors field: the Dataverse API returns []string.
// Use json.RawMessage to be robust against shape changes.
func parseAuthors(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	// Try []string first (the common case).
	var ss []string
	if err := json.Unmarshal(raw, &ss); err == nil {
		return dedup(ss)
	}
	// Fall back to []map[string]interface{} and extract "name".
	var ms []map[string]interface{}
	if err := json.Unmarshal(raw, &ms); err == nil {
		out := make([]string, 0, len(ms))
		for _, m := range ms {
			if n, ok := m["name"].(string); ok && n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

// dedup removes consecutive duplicate strings (the API sometimes repeats authors).
func dedup(ss []string) []string {
	if len(ss) == 0 {
		return ss
	}
	out := make([]string, 0, len(ss))
	seen := make(map[string]bool, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// buildURL constructs the search API URL from the given parameters.
func (c *Client) buildURL(query string, limit, offset int) string {
	q := url.QueryEscape(query)
	return fmt.Sprintf("%s?q=%s&type=dataset&per_page=%d&start=%d&sort=date&order=desc",
		c.cfg.BaseURL, q, limit, offset)
}

// SearchDatasets queries the Dataverse search API and returns matching datasets
// plus the total count.
func (c *Client) SearchDatasets(ctx context.Context, query string, limit, offset int) ([]*Dataset, int, error) {
	if limit <= 0 {
		limit = 20
	}
	rawURL := c.buildURL(query, limit, offset)
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}
	return parseSearchResp(body)
}

// RecentDatasets returns the most recently published datasets by searching
// for all records ordered by date.
func (c *Client) RecentDatasets(ctx context.Context, limit int) ([]*Dataset, int, error) {
	return c.SearchDatasets(ctx, "*", limit, 0)
}

// parseSearchResp decodes a raw Dataverse search JSON response.
func parseSearchResp(body []byte) ([]*Dataset, int, error) {
	var resp wireResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode search response: %w", err)
	}
	if resp.Status != "OK" {
		return nil, 0, fmt.Errorf("dataverse status %q", resp.Status)
	}
	out := make([]*Dataset, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		// Skip non-dataset items in mixed results.
		if item.Type != "" && item.Type != "dataset" {
			continue
		}
		out = append(out, datasetFromWire(item))
	}
	return out, resp.Data.TotalCount, nil
}

// trimDOI returns input with common DOI URL prefixes removed, leaving a bare
// "doi:…" identifier.
func trimDOI(input string) string {
	input = strings.TrimSpace(input)
	for _, pfx := range []string{"https://doi.org/", "http://doi.org/", "doi.org/"} {
		if strings.HasPrefix(input, pfx) {
			return "doi:" + strings.TrimPrefix(input, pfx)
		}
	}
	return input
}
