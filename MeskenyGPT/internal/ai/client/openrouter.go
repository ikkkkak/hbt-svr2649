package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is the minimal structure we send to OpenRouter.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse holds the model's reply.
type ChatResponse struct {
	Content string
}

// OpenRouterClient defines the interface used by the AI service.
type OpenRouterClient interface {
	Chat(ctx context.Context, messages []Message) (ChatResponse, error)
	ChatStream(ctx context.Context, messages []Message, onDelta func(delta string) error) (string, error)
}

type client struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewOpenRouterClient creates a real HTTP client for OpenRouter.
func NewOpenRouterClient(apiKey, model string, timeoutSeconds int) OpenRouterClient {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	return &client{
		apiKey: apiKey,
		model:  model,
		http: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type chatResponseWire struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type streamChunkWire struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// Chat calls the OpenRouter chat completions API.
func (c *client) Chat(ctx context.Context, messages []Message) (ChatResponse, error) {
	full, err := c.ChatStream(ctx, messages, nil)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{Content: full}, nil
}

// ChatStream streams token deltas; onDelta may be nil (collect only).
func (c *client) ChatStream(ctx context.Context, messages []Message, onDelta func(delta string) error) (string, error) {
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   onDelta != nil,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal openrouter request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create openrouter request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Title", "MeskenyGPT")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("send openrouter request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter status %d", resp.StatusCode)
	}

	if onDelta == nil {
		var wire chatResponseWire
		if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
			return "", fmt.Errorf("decode openrouter response: %w", err)
		}
		if len(wire.Choices) == 0 {
			return "", nil
		}
		return wire.Choices[0].Message.Content, nil
	}

	var full strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" {
			break
		}
		var chunk streamChunkWire
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		full.WriteString(delta)
		if err := onDelta(delta); err != nil {
			return full.String(), err
		}
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		return full.String(), fmt.Errorf("read openrouter stream: %w", err)
	}
	return full.String(), nil
}
