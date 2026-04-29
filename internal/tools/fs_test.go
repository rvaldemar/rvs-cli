package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRead_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(readArgs{FilePath: path})
	res := runRead(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "alpha") || !strings.Contains(res.Output, "1\talpha") {
		t.Errorf("expected line-numbered alpha in output, got: %q", res.Output)
	}
}

func TestRunRead_Missing(t *testing.T) {
	args, _ := json.Marshal(readArgs{FilePath: "/this/path/does/not/exist/probably-true.txt"})
	res := runRead(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError on missing file")
	}
}

func TestRunRead_OffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "many.txt")
	var b strings.Builder
	for i := 1; i <= 50; i++ {
		b.WriteString("line\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(readArgs{FilePath: path, Offset: 10, Limit: 5})
	res := runRead(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "11\tline") {
		t.Errorf("expected line 11 in output, got: %q", res.Output)
	}
	if strings.Contains(res.Output, "16\tline") {
		t.Errorf("limit not honored, output: %q", res.Output)
	}
}

func TestRunWriteThenRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	wargs, _ := json.Marshal(writeArgs{FilePath: path, Content: "hello world\n"})
	if res := runWrite(context.Background(), wargs); res.IsError {
		t.Fatalf("write failed: %s", res.Output)
	}
	rargs, _ := json.Marshal(readArgs{FilePath: path})
	res := runRead(context.Background(), rargs)
	if res.IsError {
		t.Fatalf("read failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "hello world") {
		t.Errorf("expected hello world, got %q", res.Output)
	}
}

func TestRunEdit_UniqueMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo bar baz\n"), 0o644)

	args, _ := json.Marshal(editArgs{FilePath: path, OldString: "bar", NewString: "BAR"})
	res := runEdit(context.Background(), args)
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Output)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "foo BAR baz\n" {
		t.Errorf("unexpected file content: %q", string(got))
	}
}

func TestRunEdit_MissingMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo bar\n"), 0o644)

	args, _ := json.Marshal(editArgs{FilePath: path, OldString: "qux", NewString: "X"})
	res := runEdit(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError when old_string is missing")
	}
}

func TestRunEdit_MultipleMatchesRequiresReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo foo foo\n"), 0o644)

	args, _ := json.Marshal(editArgs{FilePath: path, OldString: "foo", NewString: "bar"})
	res := runEdit(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError with multiple matches and replace_all=false")
	}

	args, _ = json.Marshal(editArgs{FilePath: path, OldString: "foo", NewString: "bar", ReplaceAll: true})
	res = runEdit(context.Background(), args)
	if res.IsError {
		t.Fatalf("replace_all should succeed: %s", res.Output)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "bar bar bar\n" {
		t.Errorf("unexpected content: %q", string(got))
	}
}

func TestRunGlob(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "sub/c.go", "sub/d.txt"} {
		full := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("x"), 0o644)
	}
	args, _ := json.Marshal(globArgs{Pattern: "**/*.go", Path: dir})
	res := runGlob(context.Background(), args)
	if res.IsError {
		t.Fatalf("glob errored: %s", res.Output)
	}
	for _, name := range []string{"a.go", "b.go", "sub/c.go"} {
		if !strings.Contains(res.Output, filepath.Join(dir, name)) {
			t.Errorf("expected %s in output, got %q", name, res.Output)
		}
	}
	if strings.Contains(res.Output, "d.txt") {
		t.Errorf("d.txt should not match *.go, got %q", res.Output)
	}
}

func TestRunGrep_FilesWithMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("nope\n"), 0o644)
	args, _ := json.Marshal(grepArgs{Pattern: "hello", Path: dir})
	res := runGrep(context.Background(), args)
	if res.IsError {
		t.Fatalf("grep errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "a.txt") || strings.Contains(res.Output, "b.txt") {
		t.Errorf("unexpected grep output: %q", res.Output)
	}
}

func TestRunGrep_ContentWithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("first\nhello\nthird\n"), 0o644)
	args, _ := json.Marshal(grepArgs{Pattern: "hello", Path: dir, OutputMode: "content", LineNumber: true})
	res := runGrep(context.Background(), args)
	if res.IsError {
		t.Fatalf("grep errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, ":2:hello") {
		t.Errorf("expected line 2 in content output, got %q", res.Output)
	}
}

func TestRunGrep_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644)
	args, _ := json.Marshal(grepArgs{Pattern: "hello", Path: dir, Glob: "*.go"})
	res := runGrep(context.Background(), args)
	if res.IsError {
		t.Fatalf("grep errored: %s", res.Output)
	}
	if !strings.Contains(res.Output, "a.go") || strings.Contains(res.Output, "a.txt") {
		t.Errorf("glob filter not honored: %q", res.Output)
	}
}
