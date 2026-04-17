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
	geminiBaseURL      = "https://generativelanguage.googleapis.com/v1beta"
	geminiDefaultModel = "gemini-1.5-flash"
)

type Gemini struct {
	apiKey string
	client *http.Client
}

func NewGemini(apiKey string) *Gemini {
	return &Gemini{
		apiKey: apiKey,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (g *Gemini) Name() string {
	return "gemini"
}

func (g *Gemini) Transform(ctx context.Context, input *TransformInput) (*TransformOutput, error) {
	model := input.Model
	if model == "" {
		model = geminiDefaultModel
	}

	parts := []map[string]interface{}{
		{"text": input.Prompt},
	}

	if input.ImageURL != "" {
		parts = append(parts, map[string]interface{}{
			"file_data": map[string]interface{}{
				"mime_type": "image/jpeg",
				"file_uri":  input.ImageURL,
			},
		})
	}

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": parts,
			},
		},
	}

	if params := input.Params; params != nil {
		genConfig := make(map[string]interface{})
		for k, v := range params {
			genConfig[k] = v
		}
		payload["generationConfig"] = genConfig
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", geminiBaseURL, model, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("gemini: unmarshal error: %w", err)
	}

	text := extractGeminiText(result)

	return &TransformOutput{
		ResultURL: "",
		Provider:  g.Name(),
		Model:     model,
		Metadata: map[string]interface{}{
			"text":     text,
			"response": result,
		},
	}, nil
}

func (g *Gemini) HealthCheck(ctx context.Context) *HealthStatus {
	url := fmt.Sprintf("%s/models?key=%s", geminiBaseURL, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HealthStatus{Provider: g.Name(), Active: false, Message: err.Error()}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthStatus{Provider: g.Name(), Active: false, Message: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &HealthStatus{Provider: g.Name(), Active: false, Message: "invalid API key"}
	}
	if resp.StatusCode == http.StatusBadRequest {
		return &HealthStatus{Provider: g.Name(), Active: false, Message: "invalid API key format"}
	}

	return &HealthStatus{Provider: g.Name(), Active: true, Message: "API key valid"}
}

func extractGeminiText(result map[string]interface{}) string {
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return ""
	}
	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return ""
	}
	part, ok := parts[0].(map[string]interface{})
	if !ok {
		return ""
	}
	text, _ := part["text"].(string)
	return text
}
