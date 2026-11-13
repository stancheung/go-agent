package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/extensions/tools"
	"go-agent/internal/ai"
)

func newTestAgent(t *testing.T, handler http.HandlerFunc) *Agent {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(context.Background(), ai.NewProvider(context.Background(), srv.URL, "test-model", ""))
}

func TestRunCarriesHistoryAcrossTurns(t *testing.T) {
	var turns [][]ai.Message
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		var req ai.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		turns = append(turns, req.Messages)
		json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{
			{Message: &ai.Message{Content: "reply"}},
		}})
	})

	if _, err := ag.Run(context.Background(), "first"); err != nil {
		t.Fatalf("Run #1: %v", err)
	}
	if _, err := ag.Run(context.Background(), "second"); err != nil {
		t.Fatalf("Run #2: %v", err)
	}

	if len(turns) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(turns))
	}
	if len(turns[0]) != 2 || turns[0][0].Role != "system" || turns[0][1].Role != "user" || turns[0][1].Content != "first" {
		t.Errorf("turn 1 = %+v, want system message then user message %q", turns[0], "first")
	}
	if len(turns[1]) != 4 {
		t.Fatalf("turn 2 history = %+v, want system/user/assistant/user", turns[1])
	}
	if turns[1][2].Role != "assistant" || turns[1][2].Content != "reply" {
		t.Errorf("turn 2 assistant = %+v, want role assistant content %q", turns[1][2], "reply")
	}
	if turns[1][3].Content != "second" {
		t.Errorf("turn 2 final = %+v, want user message %q", turns[1][3], "second")
	}
	if len(ag.History) != 4 {
		t.Errorf("History = %d messages, want 4", len(ag.History))
	}
}

func TestRunRollsBackHistoryOnFailure(t *testing.T) {
	var seen []ai.Message
	fail := true
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		var req ai.Request
		json.NewDecoder(r.Body).Decode(&req)
		seen = req.Messages
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{
			{Message: &ai.Message{Content: "ok"}},
		}})
	})

	if _, err := ag.Run(context.Background(), "doomed"); err == nil {
		t.Fatal("want error when server fails, got nil")
	}
	if len(ag.History) != 0 {
		t.Errorf("History = %+v, want empty after rollback", ag.History)
	}

	fail = false
	if _, err := ag.Run(context.Background(), "retry"); err != nil {
		t.Fatalf("Run after failure: %v", err)
	}
	if len(seen) != 2 || seen[0].Role != "system" || seen[1].Content != "retry" {
		t.Errorf("retry request = %+v, want system message then only the new user message", seen)
	}
}

func TestRunExecutesToolCalls(t *testing.T) {
	target := filepath.Join(t.TempDir(), "notes.txt")

	var requests []ai.Request
	calls := 0
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		var req ai.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, req)
		calls++

		if calls == 1 {
			json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{{
				Message: &ai.Message{
					Role: "assistant",
					ToolCalls: []ai.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: ai.ToolFunction{
							Name:      "write_file",
							Arguments: fmt.Sprintf(`{"path":%q,"content":"hi"}`, target),
						},
					}},
				},
			}}})
			return
		}
		json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{
			{Message: &ai.Message{Content: "wrote it"}},
		}})
	})
	ag.RegisterTool(tools.WriteTool{})

	reply, err := ag.Run(context.Background(), "write a note")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reply != "wrote it" {
		t.Errorf("reply = %q, want %q", reply, "wrote it")
	}
	if calls != 2 {
		t.Errorf("server saw %d requests, want 2 (tool round-trip)", calls)
	}

	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Function.Name != "write_file" {
		t.Errorf("first request tools = %+v, want write_file", requests[0].Tools)
	}
	if requests[0].ToolChoice != "auto" {
		t.Errorf("ToolChoice = %v, want \"auto\"", requests[0].ToolChoice)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("tool did not write the file: %v", err)
	}
	if string(data) != "hi" {
		t.Errorf("file = %q, want %q", data, "hi")
	}

	var toolMsg *ai.Message
	for i := range ag.History {
		if ag.History[i].Role == "tool" {
			toolMsg = &ag.History[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("history has no tool-role message")
	}
	if toolMsg.ToolCallID == nil || *toolMsg.ToolCallID != "call_1" {
		t.Errorf("tool message ToolCallID = %v, want call_1", toolMsg.ToolCallID)
	}
	if !strings.Contains(toolMsg.Content, "2 bytes") {
		t.Errorf("tool result = %q, want byte count", toolMsg.Content)
	}
}

func TestRunFeedsToolArgumentAndExecuteErrorsBack(t *testing.T) {
	calls := 0
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		var req ai.Request
		json.NewDecoder(r.Body).Decode(&req)
		calls++
		if calls == 1 {
			// Bad arguments JSON -> the tool reports a parse error, which must
			// be fed back to the model rather than failing the run.
			json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{{
				Message: &ai.Message{ToolCalls: []ai.ToolCall{{
					ID: "call_1", Type: "function",
					Function: ai.ToolFunction{Name: "write_file", Arguments: "not json"},
				}}},
			}}})
			return
		}
		json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{
			{Message: &ai.Message{Content: "recovered"}},
		}})
	})
	ag.RegisterTool(tools.WriteTool{})

	reply, err := ag.Run(context.Background(), "write")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reply != "recovered" {
		t.Errorf("reply = %q, want %q", reply, "recovered")
	}
	if calls != 2 {
		t.Errorf("server saw %d requests, want 2", calls)
	}
}

func TestRunStopsAfterToolLoopLimit(t *testing.T) {
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{{
			Message: &ai.Message{ToolCalls: []ai.ToolCall{{
				ID: "call_1", Type: "function",
				Function: ai.ToolFunction{Name: "no_such_tool", Arguments: "{}"},
			}}},
		}}})
	})

	_, err := ag.Run(context.Background(), "loop")
	if err == nil || !strings.Contains(err.Error(), "tool loop") {
		t.Errorf("err = %v, want tool-loop error", err)
	}
	if len(ag.History) != 0 {
		t.Errorf("History = %+v, want empty after rollback", ag.History)
	}
}

func TestRunRejectsEmptyReply(t *testing.T) {
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ai.Response{Choices: []ai.Choice{
			{Message: &ai.Message{Content: ""}},
		}})
	})
	_, err := ag.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if len(ag.History) != 0 {
		t.Errorf("History = %+v, want empty after rollback", ag.History)
	}
}

func TestRunRejectsResponseWithNoChoices(t *testing.T) {
	ag := newTestAgent(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ai.Response{})
	})
	_, err := ag.Run(context.Background(), "hi")
	if err == nil || !strings.Contains(fmt.Sprint(err), "no choices") {
		t.Errorf("err = %v, want a 'no choices' error", err)
	}
}

func TestDefaultSystemPromptIncludesFileRules(t *testing.T) {
	prompt := defaultSystemPrompt()
	for _, want := range []string{"File rules:", "tmp/", "project root"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}
