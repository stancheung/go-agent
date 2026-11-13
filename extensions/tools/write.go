package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteTool struct{}

func (WriteTool) Name() string { return "write_file" }
func (WriteTool) Description() string {
	return "Create or overwrite a file with the given content, creating parent directories as needed."
}
func (WriteTool) Parameters() any {
	return Schema([]string{"path", "content"}, map[string]any{
		"path":    StrParam("Absolute path of the file to write"),
		"content": StrParam("Full content to write to the file"),
	})
}

func (WriteTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("missing required argument: path")
	}

	if dir := filepath.Dir(args.Path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create parent directories for %s: %w", args.Path, err)
		}
	}
	if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", args.Path, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}
