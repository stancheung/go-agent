package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// requestTimeout bounds a single chat completion call so a hung endpoint
// cannot block the REPL loop forever.
const requestTimeout = 2 * time.Minute

var httpClient = &http.Client{Timeout: requestTimeout}

// NewProvider builds a provider for an OpenAI-compatible endpoint.
// apiKey is optional and only sent when non-empty.
func NewProvider(ctx context.Context, baseUrl string, model string, apiKey string) *Provider {
	return &Provider{BaseURL: baseUrl, Model: model, APIKey: apiKey}
}

// ChatCompletion posts one non-streaming turn to /chat/completions. req.Model
// defaults to the provider's model when left empty.
func (p *Provider) ChatCompletion(ctx context.Context, req Request) (*Response, error) {
	if req.Model == "" {
		req.Model = p.Model
	}

	// msgPayload, err := json.Marshal(req.Messages)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println(string(msgPayload))

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := httpClient.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s returned %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}

	var out Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}
