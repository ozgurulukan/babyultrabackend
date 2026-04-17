package provider

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

const (
	falBaseURL      = "https://fal.run"
	falDefaultModel = "fal-ai/flux/dev/image-to-image"
)

type FalAI struct {
	apiKey string
	client *http.Client
}

func NewFalAI(apiKey string) *FalAI {
	return &FalAI{
		apiKey: apiKey,
		client: &http.Client{Timeout: 300 * time.Second},
	}
}

func (f *FalAI) Name() string {
	return "fal.ai"
}

func (f *FalAI) Transform(ctx context.Context, input *TransformInput) (*TransformOutput, error) {
	model := input.Model
	if model == "" {
		model = falDefaultModel
	}

	payload := buildFalPayload(model, input)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: marshal error: %w", err)
	}

	url := fmt.Sprintf("%s/%s", falBaseURL, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fal.ai: request error: %w", err)
	}

	req.Header.Set("Authorization", "Key "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fal.ai: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("fal.ai: unmarshal error: %w", err)
	}

	resultURL := extractResultURL(result)

	return &TransformOutput{
		ResultURL: resultURL,
		Provider:  f.Name(),
		Model:     model,
		Metadata:  result,
	}, nil
}

func (f *FalAI) HealthCheck(ctx context.Context) *HealthStatus {
	url := "https://queue.fal.run/fal-ai/flux/dev"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HealthStatus{Provider: f.Name(), Active: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Key "+f.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthStatus{Provider: f.Name(), Active: false, Message: "connection failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &HealthStatus{Provider: f.Name(), Active: false, Message: "invalid API key"}
	}

	return &HealthStatus{Provider: f.Name(), Active: true, Message: "API key valid"}
}

// buildFalPayload constructs the API payload with model-specific field mappings.
// Different fal.ai models expect different field names and formats.
func buildFalPayload(model string, input *TransformInput) map[string]interface{} {
	payload := map[string]interface{}{
		"prompt": input.Prompt,
	}

	imageURLs := input.ImageURLs
	if len(imageURLs) == 0 && input.ImageURL != "" {
		imageURLs = []string{input.ImageURL}
	}

	switch {
	case isPuLIDModel(model):
		payload["reference_image_url"] = input.ImageURL
	case isImageURLsModel(model):
		payload["image_urls"] = imageURLs
	case isKlingVideoModel(model):
		payload["image_url"] = input.ImageURL
	default:
		payload["image_url"] = input.ImageURL
	}

	for k, v := range input.Params {
		if k == "aspect_ratio" {
			applyAspectRatio(payload, model, fmt.Sprintf("%v", v))
			continue
		}
		payload[k] = v
	}

	return payload
}

// applyAspectRatio converts our standard ratio format to the model-specific format.
// PuLID uses image_size enum; Kling uses aspect_ratio string directly.
func applyAspectRatio(payload map[string]interface{}, model, ratio string) {
	if _, exists := payload["image_size"]; exists {
		return
	}
	if _, exists := payload["aspect_ratio"]; exists {
		return
	}

	switch {
	case isPuLIDModel(model):
		if mapped := ratioToFalImageSize(ratio); mapped != "" {
			payload["image_size"] = mapped
		}
	case isKlingVideoModel(model):
		if mapped := ratioToKlingAspect(ratio); mapped != "" {
			payload["aspect_ratio"] = mapped
		}
	case isNativeAspectRatioModel(model):
		payload["aspect_ratio"] = ratio
	default:
		if mapped := ratioToFalImageSize(ratio); mapped != "" {
			payload["image_size"] = mapped
		}
	}
}

// ratioToFalImageSize maps our aspect ratio to fal.ai image_size enum values.
// Supported by Flux PuLID and similar Flux models.
func ratioToFalImageSize(ratio string) string {
	switch ratio {
	case "1:1":
		return "square_hd"
	case "4:5":
		return "portrait_4_3"
	case "9:16":
		return "portrait_16_9"
	case "16:9":
		return "landscape_16_9"
	case "3:4":
		return "portrait_4_3"
	case "4:3":
		return "landscape_4_3"
	default:
		return ""
	}
}

// ratioToKlingAspect maps our aspect ratio to Kling's supported values (16:9, 9:16, 1:1 only).
func ratioToKlingAspect(ratio string) string {
	switch ratio {
	case "1:1":
		return "1:1"
	case "9:16", "4:5", "3:4":
		return "9:16"
	case "16:9", "4:3":
		return "16:9"
	default:
		return "16:9"
	}
}

func isPuLIDModel(model string) bool {
	return strings.Contains(model, "pulid") || strings.Contains(model, "flux-pulid")
}

func isKlingVideoModel(model string) bool {
	return strings.Contains(model, "kling-video") || strings.Contains(model, "kling/")
}

func isImageURLsModel(model string) bool {
	return strings.Contains(model, "nano-banana") || strings.Contains(model, "gemini-image") || strings.Contains(model, "seedream")
}

func isNativeAspectRatioModel(model string) bool {
	return strings.Contains(model, "nano-banana") || strings.Contains(model, "gemini-image")
}

// extractResultURL extracts the media URL from various fal.ai response formats.
func extractResultURL(result map[string]interface{}) string {
	// Video output: { "video": { "url": "..." } }
	if video, ok := result["video"].(map[string]interface{}); ok {
		if url, ok := video["url"].(string); ok {
			return url
		}
	}

	// Image array: { "images": [{ "url": "..." }] }
	if images, ok := result["images"].([]interface{}); ok && len(images) > 0 {
		if img, ok := images[0].(map[string]interface{}); ok {
			if url, ok := img["url"].(string); ok {
				return url
			}
		}
	}

	// Single image object: { "image": { "url": "..." } }
	if image, ok := result["image"].(map[string]interface{}); ok {
		if url, ok := image["url"].(string); ok {
			return url
		}
	}

	// Plain string output
	if url, ok := result["output"].(string); ok {
		return url
	}

	return ""
}
