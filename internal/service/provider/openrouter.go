package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	openrouterBaseURL    = "https://openrouter.ai/api/v1"
	openrouterDefaultModel = "openai/gpt-4o"
)

type OpenRouter struct {
	apiKey string
	client *http.Client
}

func NewOpenRouter(apiKey string) *OpenRouter {
	return &OpenRouter{
		apiKey: apiKey,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (o *OpenRouter) Name() string {
	return "openrouter"
}

func (o *OpenRouter) Transform(ctx context.Context, input *TransformInput) (*TransformOutput, error) {
	model := input.Model
	if model == "" {
		model = openrouterDefaultModel
	}

	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []map[string]interface{}{
				{"type": "text", "text": input.Prompt},
				{"type": "image_url", "image_url": map[string]string{"url": input.ImageURL}},
			},
		},
	}

	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	for k, v := range input.Params {
		payload[k] = v
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", openrouterBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter: request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/ozgurulukan/babyultrabackend")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openrouter: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("openrouter: unmarshal error: %w", err)
	}

	return &TransformOutput{
		ResultURL: "",
		Provider:  o.Name(),
		Model:     model,
		Metadata:  result,
	}, nil
}

func (o *OpenRouter) HealthCheck(ctx context.Context) *HealthStatus {
	url := fmt.Sprintf("%s/auth/key", openrouterBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HealthStatus{Provider: o.Name(), Active: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthStatus{Provider: o.Name(), Active: false, Message: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return &HealthStatus{Provider: o.Name(), Active: false, Message: "invalid API key"}
	}

	return &HealthStatus{Provider: o.Name(), Active: true, Message: "API key valid"}
}
