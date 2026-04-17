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
	replicateBaseURL    = "https://api.replicate.com/v1"
	replicateDefaultModel = "stability-ai/sdxl"
)

type Replicate struct {
	apiKey string
	client *http.Client
}

func NewReplicate(apiKey string) *Replicate {
	return &Replicate{
		apiKey: apiKey,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (r *Replicate) Name() string {
	return "replicate"
}

func (r *Replicate) Transform(ctx context.Context, input *TransformInput) (*TransformOutput, error) {
	model := input.Model
	if model == "" {
		model = replicateDefaultModel
	}

	payload := map[string]interface{}{
		"version": model,
		"input": map[string]interface{}{
			"image":  input.ImageURL,
			"prompt": input.Prompt,
		},
	}
	for k, v := range input.Params {
		payload["input"].(map[string]interface{})[k] = v
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("replicate: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/predictions", replicateBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("replicate: request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "wait")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("replicate: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("replicate: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("replicate: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("replicate: unmarshal error: %w", err)
	}

	resultURL := ""
	if output, ok := result["output"].([]interface{}); ok && len(output) > 0 {
		if u, ok := output[0].(string); ok {
			resultURL = u
		}
	}

	return &TransformOutput{
		ResultURL: resultURL,
		Provider:  r.Name(),
		Model:     model,
		Metadata:  result,
	}, nil
}

func (r *Replicate) HealthCheck(ctx context.Context) *HealthStatus {
	url := fmt.Sprintf("%s/account", replicateBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HealthStatus{Provider: r.Name(), Active: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthStatus{Provider: r.Name(), Active: false, Message: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &HealthStatus{Provider: r.Name(), Active: false, Message: "invalid API key"}
	}

	return &HealthStatus{Provider: r.Name(), Active: true, Message: "API key valid"}
}
