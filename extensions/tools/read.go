package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ReadTool struct{}

func (ReadTool) Name() string { return "read_file" }
func (ReadTool) Description() string {
	return "Read a text file and return its contents, each line prefixed with its 1-based line number."
}
func (ReadTool) Parameters() any {
	return Schema([]string{"path"}, map[string]any{
		"path":   StrParam("Absolute path of the file to read"),
		"offset": IntParam("First line to return (1-based); omit to start at the top of the file"),
		"limit":  IntParam("Maximum number of lines to return; omit or 0 for unlimited (bounded by 2000)"),
	})
}

func (ReadTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("missing required argument: path")
	}

	info, err := os.Stat(args.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory; point read_file at a regular file", args.Path)
	}

	f, err := os.Open(args.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	start := args.Offset
	if start < 1 {
		start = 1
	}
	limit := args.Limit
	if limit <= 0 || limit > maxReadLines {
		limit = maxReadLines
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)

	var out strings.Builder
	size := 0
	lineNo := 0
	count := 0
	for sc.Scan() {
		lineNo++
		if lineNo < start {
			continue
		}
		count++
		if count > limit {
			break
		}
		line := fmt.Sprintf("%d: %s\n", lineNo, sc.Text())
		if size+len(line) > maxReadBytes {
			out.WriteString("... (output truncated)\n")
			break
		}
		size += len(line)
		out.WriteString(line)
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", args.Path, err)
	}
	return out.String(), nil
}
