package dataverse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const searchRespJSON = `{
	"status": "OK",
	"data": {
		"total_count": 42,
		"start": 0,
		"per_page": 10,
		"items": [
			{
				"name": "Climate Model Output 2020",
				"type": "dataset",
				"url": "https://doi.org/10.7910/DVN/AAAA01",
				"global_id": "doi:10.7910/DVN/AAAA01",
				"description": "Global temperature data from 2020 climate runs.",
				"published_at": "2023-01-15T10:00:00Z",
				"publisher": "Harvard Dataverse",
				"subjects": ["Earth and Environmental Sciences"],
				"fileCount": 3,
				"createdAt": "2022-11-01T08:00:00Z",
				"updatedAt": "2023-01-15T10:00:00Z",
				"authors": ["Smith, Alice", "Jones, Bob"]
			},
			{
				"name": "Survey Data on AI Adoption",
				"type": "dataset",
				"url": "https://doi.org/10.7910/DVN/BBBB02",
				"global_id": "doi:10.7910/DVN/BBBB02",
				"description": "Annual survey on AI adoption in enterprises.",
				"published_at": "2023-03-20T14:30:00Z",
				"publisher": "IQSS Dataverse",
				"subjects": ["Computer and Information Science", "Social Sciences"],
				"fileCount": 5,
				"createdAt": "2023-02-10T09:00:00Z",
				"updatedAt": "2023-03-20T14:30:00Z",
				"authors": ["Chen, Wei", "Patel, Riya"]
			}
		]
	}
}`

func TestSearchDatasets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchRespJSON))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := NewClientWithConfig(cfg)

	datasets, total, err := c.SearchDatasets(context.Background(), "climate", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}
	if len(datasets) != 2 {
		t.Fatalf("len(datasets) = %d, want 2", len(datasets))
	}
	if datasets[0].ID != "doi:10.7910/DVN/AAAA01" {
		t.Errorf("datasets[0].ID = %q, want doi:10.7910/DVN/AAAA01", datasets[0].ID)
	}
	if datasets[0].Title != "Climate Model Output 2020" {
		t.Errorf("datasets[0].Title = %q, want Climate Model Output 2020", datasets[0].Title)
	}
	if datasets[1].ID != "doi:10.7910/DVN/BBBB02" {
		t.Errorf("datasets[1].ID = %q, want doi:10.7910/DVN/BBBB02", datasets[1].ID)
	}
	if len(datasets[0].Authors) != 2 {
		t.Errorf("datasets[0].Authors = %v, want 2 entries", datasets[0].Authors)
	}
}

func TestRecentDatasets(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","data":{"total_count":1,"start":0,"per_page":10,"items":[{"name":"New Dataset","type":"dataset","global_id":"doi:10.7910/DVN/CCCC03","url":"https://doi.org/10.7910/DVN/CCCC03","published_at":"2023-05-01T00:00:00Z","fileCount":1,"authors":[]}]}}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := NewClientWithConfig(cfg)

	datasets, _, err := c.RecentDatasets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "*" {
		t.Errorf("query sent = %q, want *", gotQuery)
	}
	if len(datasets) != 1 {
		t.Errorf("len(datasets) = %d, want 1", len(datasets))
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","data":{"total_count":1,"start":0,"per_page":10,"items":[{"name":"Recovery Dataset","type":"dataset","global_id":"doi:10.7910/DVN/RECO01","url":"https://doi.org/10.7910/DVN/RECO01","fileCount":1,"authors":[]}]}}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClientWithConfig(cfg)

	start := time.Now()
	datasets, _, err := c.SearchDatasets(context.Background(), "recovery", 10, 0)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if len(datasets) != 1 {
		t.Errorf("len(datasets) = %d, want 1", len(datasets))
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("elapsed %v, want >= 500ms (retries should back off)", elapsed)
	}
}

func TestBackoff(t *testing.T) {
	c := NewClient()
	if got := c.backoff(1); got != 500*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 500ms", got)
	}
	if got := c.backoff(10); got != 5*time.Second {
		t.Errorf("backoff(10) = %v, want 5s", got)
	}
}

func TestParseAuthorsStringSlice(t *testing.T) {
	raw := []byte(`["Smith, Alice","Jones, Bob"]`)
	got := parseAuthors(raw)
	if len(got) != 2 || got[0] != "Smith, Alice" {
		t.Errorf("parseAuthors([]string) = %v, want [Smith, Alice Jones, Bob]", got)
	}
}

func TestParseAuthorsMap(t *testing.T) {
	raw := []byte(`[{"name":"Smith, Alice"},{"name":"Jones, Bob"}]`)
	got := parseAuthors(raw)
	if len(got) != 2 || got[0] != "Smith, Alice" {
		t.Errorf("parseAuthors([]map) = %v, want [Smith, Alice Jones, Bob]", got)
	}
}

func TestBuildURL(t *testing.T) {
	c := NewClient()
	got := c.buildURL("climate change", 10, 20)
	if !strings.Contains(got, "q=climate+change") && !strings.Contains(got, "q=climate%20change") {
		t.Errorf("buildURL missing encoded query: %s", got)
	}
	if !strings.Contains(got, "per_page=10") {
		t.Errorf("buildURL missing per_page=10: %s", got)
	}
	if !strings.Contains(got, "start=20") {
		t.Errorf("buildURL missing start=20: %s", got)
	}
	if !strings.Contains(got, "sort=date") {
		t.Errorf("buildURL missing sort=date: %s", got)
	}
}
