package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runTool(t *testing.T, tool Tool, args map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return tool.Execute(context.Background(), string(raw))
}

func TestReadTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, err := runTool(t, ReadTool{}, map[string]any{"path": path})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "1: one\n2: two\n3: three\n4: four\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}

	out, err = runTool(t, ReadTool{}, map[string]any{"path": path, "offset": 3, "limit": 1})
	if err != nil {
		t.Fatalf("read slice: %v", err)
	}
	if out != "3: three\n" {
		t.Errorf("sliced out = %q, want %q", out, "3: three\n")
	}
}

func TestReadToolErrors(t *testing.T) {
	if _, err := runTool(t, ReadTool{}, map[string]any{"path": filepath.Join(t.TempDir(), "nope.txt")}); err == nil {
		t.Error("want error for missing file, got nil")
	}
	if _, err := runTool(t, ReadTool{}, map[string]any{"path": t.TempDir()}); err == nil {
		t.Error("want error for directory, got nil")
	}
	if _, err := runTool(t, ReadTool{}, map[string]any{}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Errorf("want missing-path error, got %v", err)
	}
}

func TestWriteTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "b.txt")
	out, err := runTool(t, WriteTool{}, map[string]any{"path": path, "content": "hi\n"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "bytes") || !strings.Contains(out, path) {
		t.Errorf("out = %q, want byte count and path", out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hi\n" {
		t.Errorf("file content = %q, want %q", data, "hi\n")
	}
}

func TestEditTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.txt")
	if err := os.WriteFile(path, []byte("baab"), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	out, err := runTool(t, EditTool{}, map[string]any{"path": path, "old_string": "aa", "new_string": "cc"})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "edited") {
		t.Errorf("out = %q, want confirmation", out)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "bccb" {
		t.Errorf("content = %q, want %q", data, "bccb")
	}
}

func TestEditToolErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.txt")
	if _, err := runTool(t, EditTool{}, map[string]any{"path": missing, "old_string": "x", "new_string": "y"}); err == nil {
		t.Error("want error for missing file, got nil")
	}

	ambiguous := filepath.Join(dir, "amb.txt")
	os.WriteFile(ambiguous, []byte("aba"), 0o644)
	_, err := runTool(t, EditTool{}, map[string]any{"path": ambiguous, "old_string": "a", "new_string": "c"})
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("want ambiguity error, got %v", err)
	}

	notFound := filepath.Join(dir, "nf.txt")
	os.WriteFile(notFound, []byte("zzz"), 0o644)
	_, err = runTool(t, EditTool{}, map[string]any{"path": notFound, "old_string": "qq", "new_string": "c"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want not-found error, got %v", err)
	}
}

func TestBashTool(t *testing.T) {
	out, err := runTool(t, BashTool{}, map[string]any{"command": "echo hello-unique-123"})
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	if !strings.HasPrefix(out, "exit: 0\n") {
		t.Errorf("out = %q, want leading exit: 0 line", out)
	}
	if !strings.Contains(out, "hello-unique-123") {
		t.Errorf("out = %q, want echo output", out)
	}

	out, err = runTool(t, BashTool{}, map[string]any{"command": "exit 3"})
	if err != nil {
		t.Fatalf("bash exit: %v", err)
	}
	if !strings.HasPrefix(out, "exit: 3\n") {
		t.Errorf("out = %q, want leading exit: 3 line", out)
	}

	if _, err := runTool(t, BashTool{}, map[string]any{}); err == nil || !strings.Contains(err.Error(), "command") {
		t.Errorf("want missing-command error, got %v", err)
	}
}

func TestShellDescription(t *testing.T) {
	desc := ShellDescription()
	if desc == "" {
		t.Fatal("empty shell description")
	}
	if runtime.GOOS == "windows" && !strings.Contains(desc, "Windows PowerShell") {
		t.Errorf("desc = %q, want Windows PowerShell with probed version", desc)
	}
	t.Logf("shell: %s", desc)
}

func TestBashToolRejectsPosixisms(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("posixism guard is Windows-only")
	}
	for _, cmd := range []string{"foo || bar", "foo && bar", "cat x 2>/dev/null"} {
		if _, err := runTool(t, BashTool{}, map[string]any{"command": cmd}); err == nil {
			t.Errorf("want rejection for %q, got nil error", cmd)
		}
	}
}
