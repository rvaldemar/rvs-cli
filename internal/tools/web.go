package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type webFetchArgs struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"` // ignored — we return raw body and let the LLM summarize
}

const (
	webFetchTimeout    = 30 * time.Second
	webFetchMaxBytes   = 200 * 1024
	webFetchMaxRedirs  = 5
	webFetchUserAgent  = "rvs-cli/code (+https://agents.rvs.solutions)"
	webFetchHelpTrunc  = "\n…(truncated; pass narrower URL to fetch less)"
)

func runWebFetch(ctx context.Context, raw json.RawMessage) Result {
	var a webFetchArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("WebFetch", err), IsError: true}
	}
	if a.URL == "" {
		return Result{Output: formatError("WebFetch", errors.New("url is required")), IsError: true}
	}
	parsed, err := url.Parse(a.URL)
	if err != nil {
		return Result{Output: formatError("WebFetch", err), IsError: true}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Result{Output: formatError("WebFetch", fmt.Errorf("unsupported scheme %q (only http/https)", parsed.Scheme)), IsError: true}
	}

	cctx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()

	client := &http.Client{
		Timeout: webFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= webFetchMaxRedirs {
				return errors.New("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(cctx, "GET", a.URL, nil)
	if err != nil {
		return Result{Output: formatError("WebFetch", err), IsError: true}
	}
	req.Header.Set("User-Agent", webFetchUserAgent)
	req.Header.Set("Accept", "text/html, text/plain, application/json;q=0.9, */*;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return Result{Output: formatError("WebFetch", err), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes+1))
	if err != nil {
		return Result{Output: formatError("WebFetch", err), IsError: true}
	}
	truncated := len(body) > webFetchMaxBytes
	if truncated {
		body = body[:webFetchMaxBytes]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "GET %s — HTTP %d %s\n", a.URL, resp.StatusCode, resp.Status)
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		fmt.Fprintf(&b, "Content-Type: %s\n", ct)
	}
	b.WriteString("\n")
	b.Write(body)
	if truncated {
		b.WriteString(webFetchHelpTrunc)
	}

	if resp.StatusCode >= 400 {
		return Result{Output: b.String(), IsError: true}
	}
	return Result{Output: b.String()}
}
