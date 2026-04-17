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
	deepseekBaseURL    = "https://api.deepseek.com/v1"
	deepseekDefaultModel = "deepseek-chat"
)

type DeepSeek struct {
	apiKey string
	client *http.Client
}

func NewDeepSeek(apiKey string) *DeepSeek {
	return &DeepSeek{
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (d *DeepSeek) Name() string {
	return "deepseek"
}

func (d *DeepSeek) Transform(ctx context.Context, input *TransformInput) (*TransformOutput, error) {
	model := input.Model
	if model == "" {
		model = deepseekDefaultModel
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
		return nil, fmt.Errorf("deepseek: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", deepseekBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deepseek: request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepseek: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("deepseek: unmarshal error: %w", err)
	}

	return &TransformOutput{
		ResultURL: "",
		Provider:  d.Name(),
		Model:     model,
		Metadata:  result,
	}, nil
}

func (d *DeepSeek) HealthCheck(ctx context.Context) *HealthStatus {
	url := fmt.Sprintf("%s/models", deepseekBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HealthStatus{Provider: d.Name(), Active: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthStatus{Provider: d.Name(), Active: false, Message: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &HealthStatus{Provider: d.Name(), Active: false, Message: "invalid API key"}
	}

	return &HealthStatus{Provider: d.Name(), Active: true, Message: "API key valid"}
}
