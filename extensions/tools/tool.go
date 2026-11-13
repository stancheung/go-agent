package tools

import "context"

// Tool is a function the agent exposes to the model, invoked via OpenAI-style
// tool_calls. Execute receives the model's JSON arguments and returns the text
// fed back as a tool result message.
type Tool interface {
	Name() string
	Description() string
	Parameters() any
	Execute(ctx context.Context, argsJSON string) (string, error)
}

const (
	// maxReadLines and maxReadBytes bound how much read_file returns.
	maxReadLines = 2000
	maxReadBytes = 51200
	// maxToolOutput bounds how much of any tool's output is returned to the model.
	maxToolOutput = 32768
	// defaultBashTimeout bounds a bash call when the model does not set one.
	defaultBashTimeout = 120
)

// Schema builds a JSON object schema for a tool's Parameters().
func Schema(required []string, properties map[string]any) any {
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// StrParam describes a string property in a tool schema.
func StrParam(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// IntParam describes an integer property in a tool schema.
func IntParam(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
