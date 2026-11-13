package ai

type Provider struct {
	BaseURL string
	Model   string
	APIKey  string
}

// Request represents an OpenAI chat completion request
type Request struct {
	Model            string          `json:"model"`
	Messages         []Message       `json:"messages"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	Stream           *bool           `json:"stream,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int  `json:"logit_bias,omitempty"`
	User             *string         `json:"user,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       any             `json:"tool_choice,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	N                *int            `json:"n,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	Logprobs         *bool           `json:"logprobs,omitempty"`
	TopLogprobs      *int            `json:"top_logprobs,omitempty"`
	ReasoningEffort  *string         `json:"reasoning_effort,omitempty"`
}

// Tool represents a tool that can be called
type Tool struct {
	Type     string   `json:"type"`
	Function ToolSpec `json:"function"`
}

// ToolSpec defines a tool specification
type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// ToolCall represents a tool function call
type ToolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents the function being called
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ResponseFormat specifies the format of the response
type ResponseFormat struct {
	Type string `json:"type"`
}

// Message represents a chat message
type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content"`
	Name             *string    `json:"name,omitempty"`
	ToolCallID       *string    `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

// Response represents an OpenAI chat completion response
// type Response struct {
// 	ID      string   `json:"id"`
// 	Object  string   `json:"object"`
// 	Created int64    `json:"created"`
// 	Model   string   `json:"model"`
// 	Choices []Choice `json:"choices"`
// 	Usage   Usage    `json:"usage"`
// }

// Response is one chat completion result: the whole body when Stream was
// false, or a single SSE chunk (Object == "chat.completion.chunk") when true.
// Check Object, or which of Choice.Message/Choice.Delta is non-nil.
type Response struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"` // streaming: only on the last chunk, if stream_options.include_usage
}

// Choice represents a choice in the response
// type Choice struct {
// 	Index        int     `json:"index"`
// 	Message      Message `json:"message"`
// 	FinishReason *string `json:"finish_reason"`
// }

// Choice carries Message in a non-streaming response and Delta in a stream.
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason *string  `json:"finish_reason"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// EmbeddingRequest represents an OpenAI embedding request
type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
	Dimensions     *int     `json:"dimensions,omitempty"`
	User           *string  `json:"user,omitempty"`
}

// EmbeddingResponse represents an OpenAI embedding response
type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

// EmbeddingData represents a single embedding
type EmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// EmbeddingUsage represents token usage for embeddings
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
