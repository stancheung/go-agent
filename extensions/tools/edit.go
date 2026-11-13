package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type EditTool struct{}

func (EditTool) Name() string { return "edit_file" }
func (EditTool) Description() string {
	return "Replace an exact string in a file with a new string. old_string must match the file exactly and appear exactly once."
}
func (EditTool) Parameters() any {
	return Schema([]string{"path", "old_string", "new_string"}, map[string]any{
		"path":       StrParam("Absolute path of the file to edit"),
		"old_string": StrParam("Exact text already in the file that must appear exactly once"),
		"new_string": StrParam("Replacement text to put in its place"),
	})
}

func (EditTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("missing required argument: path")
	}
	if args.OldString == "" {
		return "", fmt.Errorf("missing required argument: old_string")
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", err
	}
	content := string(data)

	idx := strings.Index(content, args.OldString)
	if idx < 0 {
		return "", fmt.Errorf("old_string not found in %s; read the file first to get its exact contents", args.Path)
	}
	if strings.Contains(content[idx+len(args.OldString):], args.OldString) {
		return "", fmt.Errorf("old_string matches more than once in %s; include more surrounding context", args.Path)
	}

	content = content[:idx] + args.NewString + content[idx+len(args.OldString):]
	if err := os.WriteFile(args.Path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", args.Path, err)
	}
	return fmt.Sprintf("edited %s", args.Path), nil
}
