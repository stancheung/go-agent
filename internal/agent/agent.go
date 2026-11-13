package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"go-agent/extensions/tools"
	"go-agent/internal/ai"
)

// maxToolIterations caps how many LLM round-trips a single Run may make while
// the model keeps requesting tool calls, guarding against loops.
const maxToolIterations = 30

type Agent struct {
	Provider *ai.Provider
	History  []ai.Message
	Tools    map[string]tools.Tool
	// SystemPrompt is sent as a leading system message on every request. It is
	// kept out of History so rollback and replay only ever touch the dialogue.
	SystemPrompt string
}

func New(ctx context.Context, provider *ai.Provider) *Agent {
	readTool := tools.ReadTool{}
	writeTool := tools.WriteTool{}
	editTool := tools.EditTool{}
	bashTool := tools.BashTool{}
	return &Agent{
		Provider: provider,
		Tools: map[string]tools.Tool{
			readTool.Name():  readTool,
			writeTool.Name(): writeTool,
			editTool.Name():  editTool,
			bashTool.Name():  bashTool,
		},
		SystemPrompt: defaultSystemPrompt(),
	}
}

// defaultSystemPrompt tells the model what environment it is in so it does not
// have to guess the shell (the root cause of POSIX-isms on PowerShell).
func defaultSystemPrompt() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "unknown"
	}

	sysPrompt := fmt.Sprintf("You are go-agent, an AI assistant that can run shell commands and edit files.\n"+
		"Environment:\n"+
		"- OS: %s\n"+
		"- Working directory: %s\n"+
		"- Shell used by the bash tool: %s\n"+
		"Shell rules:\n%s\n"+
		"File rules:\n"+
		"- Any file you generate as output of a task (extracted text, converted documents, scratch scripts, reports) must be written under the tmp/ directory inside the working directory; create it first if missing. Never write generated files directly into the project root.",
		runtime.GOOS, cwd, tools.ShellDescription(), tools.ShellGuidelines(),
	)

	fmt.Printf("Agent initialized with system prompt:\n%s\n\n", sysPrompt)

	return sysPrompt
}

// RegisterTool exposes a tool to the model on subsequent Run calls.
func (a *Agent) RegisterTool(t tools.Tool) {
	a.Tools[t.Name()] = t
}

// Run sends input to the model and returns the assistant reply. The model may
// request tool calls; each is executed and its result fed back, looping until
// the model returns plain text. On any failure every message added by this Run
// is rolled back so the input can simply be retried.
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	start := len(a.History) // Snapshot of the current turn of conversation before starting a new turn
	query := ai.Message{Role: "user", Content: input}
	a.History = append(a.History, query)

	var reply *ai.Message
	for iteration := 0; ; iteration++ {
		if iteration >= maxToolIterations {
			return "", a.rollback(start, fmt.Errorf("tool loop did not finish after %d iterations", maxToolIterations))
		}

		var err error
		reply, err = a.nextReply(ctx)
		if err != nil {
			return "", a.rollback(start, err)
		}
		if reply == nil {
			return "", a.rollback(start, fmt.Errorf("response contained no choices"))
		}
		reply.Role = "assistant"
		a.History = append(a.History, *reply)

		if len(reply.ToolCalls) == 0 {
			content := strings.TrimSpace(reply.Content)
			if content == "" {
				return "", a.rollback(start, fmt.Errorf("model returned an empty response"))
			}
			return content, nil
		}

		results := make([]ai.Message, len(reply.ToolCalls))
		var wg sync.WaitGroup
		for i, toolCall := range reply.ToolCalls {
			wg.Go(func() {
				fmt.Printf("Tool: %s %s\n\n", toolCall.Function.Name, toolCall.Function.Arguments)
				result := a.executeTool(ctx, toolCall)
				id := toolCall.ID
				results[i] = ai.Message{Role: "tool", ToolCallID: &id, Content: result}
				// a.History = append(a.History, ai.Message{Role: "tool", ToolCallID: &id, Content: result})
			})
		}
		wg.Wait()
		a.History = append(a.History, results...)
	}
}

func (a *Agent) nextReply(ctx context.Context) (*ai.Message, error) {
	msgs := a.History
	if a.SystemPrompt != "" {
		msgs = append([]ai.Message{{Role: "system", Content: a.SystemPrompt}}, a.History...)
	}
	req := ai.Request{Messages: msgs}
	if len(a.Tools) > 0 {
		req.Tools = a.toolDefs()
		req.ToolChoice = "auto"
	}
	resp, err := a.Provider.ChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("response contained no choices")
	}
	return resp.Choices[0].Message, nil
}

// executeTool runs a model-requested tool call. Failures (unknown tool, bad
// arguments, tool error) are returned as text so the model can self-correct;
// only the enclosing Run fails on transport or loop errors.
func (a *Agent) executeTool(ctx context.Context, toolCall ai.ToolCall) string {
	t, ok := a.Tools[toolCall.Function.Name]
	if !ok {
		return fmt.Sprintf("error: no tool named %q", toolCall.Function.Name)
	}
	result, err := t.Execute(ctx, toolCall.Function.Arguments)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return result
}

func (a *Agent) toolDefs() []ai.Tool {
	names := make([]string, 0, len(a.Tools))
	for name := range a.Tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]ai.Tool, 0, len(names))
	for _, name := range names {
		t := a.Tools[name]
		defs = append(defs, ai.Tool{
			Type: "function",
			Function: ai.ToolSpec{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

func (a *Agent) rollback(start int, cause error) error {
	a.History = a.History[:start]
	return cause
}
