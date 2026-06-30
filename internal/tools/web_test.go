package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunWebFetch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	args, _ := json.Marshal(webFetchArgs{URL: srv.URL})
	res := runWebFetch(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "hello world") {
		t.Errorf("expected body in output, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "HTTP 200") {
		t.Errorf("expected status line, got %q", res.Output)
	}
}

func TestRunWebFetch_4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	args, _ := json.Marshal(webFetchArgs{URL: srv.URL})
	res := runWebFetch(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError on 404")
	}
}

func TestRunWebFetch_BadScheme(t *testing.T) {
	args, _ := json.Marshal(webFetchArgs{URL: "ftp://example.com"})
	res := runWebFetch(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError on bad scheme")
	}
}
