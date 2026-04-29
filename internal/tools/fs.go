package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// === Read ===

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

const (
	defaultReadLimit = 2000
	maxLineWidth     = 2000
)

func runRead(_ context.Context, raw json.RawMessage) Result {
	var a readArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("Read", err), IsError: true}
	}
	if a.FilePath == "" {
		return Result{Output: formatError("Read", errors.New("file_path is required")), IsError: true}
	}
	path, err := absPath(a.FilePath)
	if err != nil {
		return Result{Output: formatError("Read", err), IsError: true}
	}

	f, err := os.Open(path)
	if err != nil {
		return Result{Output: formatError("Read", err), IsError: true}
	}
	defer f.Close()

	limit := a.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var b strings.Builder
	lineNo := 0
	emitted := 0
	for scanner.Scan() {
		lineNo++
		if lineNo <= a.Offset {
			continue
		}
		line := scanner.Text()
		if len(line) > maxLineWidth {
			line = line[:maxLineWidth] + "…"
		}
		fmt.Fprintf(&b, "%6d\t%s\n", lineNo, line)
		emitted++
		if emitted >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{Output: formatError("Read", err), IsError: true}
	}
	if emitted == 0 {
		return Result{Output: "(file is empty or offset is past EOF)", IsError: false}
	}
	return Result{Output: b.String(), IsError: false}
}

// === Write ===

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func runWrite(_ context.Context, raw json.RawMessage) Result {
	var a writeArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("Write", err), IsError: true}
	}
	if a.FilePath == "" {
		return Result{Output: formatError("Write", errors.New("file_path is required")), IsError: true}
	}
	path, err := absPath(a.FilePath)
	if err != nil {
		return Result{Output: formatError("Write", err), IsError: true}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{Output: formatError("Write", err), IsError: true}
	}
	if err := os.WriteFile(path, []byte(a.Content), 0o644); err != nil {
		return Result{Output: formatError("Write", err), IsError: true}
	}
	return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(a.Content), path)}
}

// === Edit ===

type editArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func runEdit(_ context.Context, raw json.RawMessage) Result {
	var a editArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("Edit", err), IsError: true}
	}
	if a.FilePath == "" {
		return Result{Output: formatError("Edit", errors.New("file_path is required")), IsError: true}
	}
	if a.OldString == "" {
		return Result{Output: formatError("Edit", errors.New("old_string is required")), IsError: true}
	}
	if a.OldString == a.NewString {
		return Result{Output: formatError("Edit", errors.New("old_string and new_string are identical")), IsError: true}
	}
	path, err := absPath(a.FilePath)
	if err != nil {
		return Result{Output: formatError("Edit", err), IsError: true}
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return Result{Output: formatError("Edit", err), IsError: true}
	}
	src := string(bytes)
	count := strings.Count(src, a.OldString)
	if count == 0 {
		return Result{Output: formatError("Edit", errors.New("old_string not found in file")), IsError: true}
	}
	if count > 1 && !a.ReplaceAll {
		return Result{Output: formatError("Edit", fmt.Errorf("old_string matches %d places — pass replace_all=true or add more context to disambiguate", count)), IsError: true}
	}

	var out string
	if a.ReplaceAll {
		out = strings.ReplaceAll(src, a.OldString, a.NewString)
	} else {
		out = strings.Replace(src, a.OldString, a.NewString, 1)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return Result{Output: formatError("Edit", err), IsError: true}
	}
	verb := "replaced"
	if a.ReplaceAll {
		verb = fmt.Sprintf("replaced %d occurrences in", count)
	}
	return Result{Output: fmt.Sprintf("%s %s", verb, path)}
}

// === Glob ===

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

const globMaxResults = 500

func runGlob(_ context.Context, raw json.RawMessage) Result {
	var a globArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("Glob", err), IsError: true}
	}
	if a.Pattern == "" {
		return Result{Output: formatError("Glob", errors.New("pattern is required")), IsError: true}
	}
	root := a.Path
	if root == "" {
		var err error
		if root, err = os.Getwd(); err != nil {
			return Result{Output: formatError("Glob", err), IsError: true}
		}
	}
	root, err := absPath(root)
	if err != nil {
		return Result{Output: formatError("Glob", err), IsError: true}
	}

	matches, err := doublestar.Glob(os.DirFS(root), a.Pattern, doublestar.WithFilesOnly())
	if err != nil {
		return Result{Output: formatError("Glob", err), IsError: true}
	}
	if len(matches) == 0 {
		return Result{Output: "(no files matched)"}
	}

	type fileEntry struct {
		path    string
		modTime int64
	}
	entries := make([]fileEntry, 0, len(matches))
	for _, m := range matches {
		full := filepath.Join(root, m)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		entries = append(entries, fileEntry{path: full, modTime: info.ModTime().UnixNano()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].modTime > entries[j].modTime })

	if len(entries) > globMaxResults {
		entries = entries[:globMaxResults]
	}

	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.path)
		b.WriteByte('\n')
	}
	if len(matches) > globMaxResults {
		fmt.Fprintf(&b, "(truncated to %d of %d matches; narrow the pattern or path)\n", globMaxResults, len(matches))
	}
	return Result{Output: strings.TrimRight(b.String(), "\n")}
}

// === Grep ===

type grepArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	OutputMode string `json:"output_mode"` // "files_with_matches" (default) | "content" | "count"
	IgnoreCase bool   `json:"-i"`
	LineNumber bool   `json:"-n"`
	HeadLimit  int    `json:"head_limit"`
}

const grepMaxBytes = 5 * 1024 * 1024 // skip files larger than this

func runGrep(ctx context.Context, raw json.RawMessage) Result {
	var a grepArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("Grep", err), IsError: true}
	}
	if a.Pattern == "" {
		return Result{Output: formatError("Grep", errors.New("pattern is required")), IsError: true}
	}

	flags := ""
	if a.IgnoreCase {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + a.Pattern)
	if err != nil {
		return Result{Output: formatError("Grep", fmt.Errorf("invalid regex: %w", err)), IsError: true}
	}

	root := a.Path
	if root == "" {
		var err error
		if root, err = os.Getwd(); err != nil {
			return Result{Output: formatError("Grep", err), IsError: true}
		}
	}
	root, err = absPath(root)
	if err != nil {
		return Result{Output: formatError("Grep", err), IsError: true}
	}

	mode := a.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}

	var (
		files     []string
		matches   []string // for content mode: "path:line:content" or "path:content"
		count     int
		truncated bool
	)
	limit := a.HeadLimit
	if limit <= 0 {
		limit = 200
	}

	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if a.Glob != "" {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				rel = p
			}
			match, err := doublestar.Match(a.Glob, rel)
			if err != nil || !match {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > grepMaxBytes {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		fileMatched := false
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !re.MatchString(line) {
				continue
			}
			fileMatched = true
			count++
			switch mode {
			case "content":
				if a.LineNumber {
					matches = append(matches, fmt.Sprintf("%s:%d:%s", p, lineNo, line))
				} else {
					matches = append(matches, fmt.Sprintf("%s:%s", p, line))
				}
			}
			if mode == "content" && len(matches) >= limit {
				truncated = true
				return io.EOF
			}
			if mode == "files_with_matches" {
				break
			}
		}
		if mode == "files_with_matches" && fileMatched {
			files = append(files, p)
			if len(files) >= limit {
				truncated = true
				return io.EOF
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, io.EOF) && !errors.Is(walkErr, context.Canceled) {
		return Result{Output: formatError("Grep", walkErr), IsError: true}
	}

	var b strings.Builder
	switch mode {
	case "files_with_matches":
		if len(files) == 0 {
			return Result{Output: "(no files matched)"}
		}
		sort.Strings(files)
		for _, f := range files {
			b.WriteString(f)
			b.WriteByte('\n')
		}
	case "content":
		if len(matches) == 0 {
			return Result{Output: "(no matches)"}
		}
		for _, m := range matches {
			b.WriteString(m)
			b.WriteByte('\n')
		}
	case "count":
		fmt.Fprintf(&b, "%d match(es)\n", count)
	default:
		return Result{Output: formatError("Grep", fmt.Errorf("unknown output_mode %q", a.OutputMode)), IsError: true}
	}
	if truncated {
		fmt.Fprintf(&b, "(truncated at %d entries; pass a higher head_limit if you need more)\n", limit)
	}
	return Result{Output: strings.TrimRight(b.String(), "\n")}
}

func absPath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}
