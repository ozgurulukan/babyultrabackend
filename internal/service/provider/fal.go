package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	falBaseURL      = "https://fal.run"
	falQueueURL     = "https://queue.fal.run"
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

	log.Printf("fal.ai payload for model %s: %s", model, string(body))

	if isQueueModel(model) {
		return f.transformViaQueue(ctx, model, payload)
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

// transformViaQueue submits a request via fal.ai's queue API and polls for the result.
// This is required for long-running models like Kling video that use async processing.
func (f *FalAI) transformViaQueue(ctx context.Context, model string, payload map[string]interface{}) (*TransformOutput, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: marshal error: %w", err)
	}

	submitURL := fmt.Sprintf("%s/%s", falQueueURL, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fal.ai: queue submit request error: %w", err)
	}
	req.Header.Set("Authorization", "Key "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: queue submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: queue submit read error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("fal.ai queue submit error for model %s (status %d): %s", model, resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("fal.ai: queue submit error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var submitResult map[string]interface{}
	if err := json.Unmarshal(respBody, &submitResult); err != nil {
		return nil, fmt.Errorf("fal.ai: queue submit unmarshal error: %w", err)
	}

	requestID, _ := submitResult["request_id"].(string)
	if requestID == "" {
		return nil, fmt.Errorf("fal.ai: queue submit did not return request_id")
	}

	// Use status_url and response_url from submit response if available
	// (fal.ai returns the correct polling URLs in the response)
	statusURL, _ := submitResult["status_url"].(string)
	resultURL, _ := submitResult["response_url"].(string)
	if statusURL == "" {
		statusURL = fmt.Sprintf("%s/%s/requests/%s/status", falQueueURL, model, requestID)
	}
	if resultURL == "" {
		resultURL = fmt.Sprintf("%s/%s/requests/%s", falQueueURL, model, requestID)
	}

	log.Printf("fal.ai queue submitted model=%s request_id=%s status_url=%s response_url=%s", model, requestID, statusURL, resultURL)

	pollClient := &http.Client{Timeout: 120 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("fal.ai: queue poll cancelled: %w", ctx.Err())
		default:
		}

		sReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return nil, fmt.Errorf("fal.ai: queue status request error: %w", err)
		}
		sReq.Header.Set("Authorization", "Key "+f.apiKey)
		sReq.Header.Set("Accept", "application/json")

		sResp, err := pollClient.Do(sReq)
		if err != nil {
			return nil, fmt.Errorf("fal.ai: queue status poll failed: %w", err)
		}
		sBody, sReadErr := io.ReadAll(sResp.Body)
		sResp.Body.Close()

		if sResp.StatusCode != http.StatusOK {
			log.Printf("fal.ai queue status error (status %d): %s", sResp.StatusCode, string(sBody))
			return nil, fmt.Errorf("fal.ai: queue status error (status %d): %s", sResp.StatusCode, string(sBody))
		}

		if sReadErr != nil {
			return nil, fmt.Errorf("fal.ai: queue status read error: %w", sReadErr)
		}

		if len(sBody) == 0 {
			log.Printf("fal.ai queue status returned empty body for request %s, retrying...", requestID)
			time.Sleep(3 * time.Second)
			continue
		}

		var status map[string]interface{}
		if err := json.Unmarshal(sBody, &status); err != nil {
			log.Printf("fal.ai queue status unmarshal error for request %s: %v, body: %s", requestID, err, string(sBody))
			return nil, fmt.Errorf("fal.ai: queue status unmarshal error: %w", err)
		}

		statusStr, _ := status["status"].(string)
		log.Printf("fal.ai queue status for request %s: %s", requestID, statusStr)
		switch statusStr {
		case "COMPLETED":
			goto fetchResult
		case "IN_PROGRESS":
			time.Sleep(5 * time.Second)
			continue
		case "IN_QUEUE":
			time.Sleep(5 * time.Second)
			continue
		case "FAILED":
			errMsg, _ := status["error"].(string)
			if errMsg == "" {
				errMsg = string(sBody)
			}
			return nil, fmt.Errorf("fal.ai: queue request failed: %s", errMsg)
		default:
			if strings.HasPrefix(strings.ToUpper(statusStr), "IN_") {
				time.Sleep(3 * time.Second)
				continue
			}
			return nil, fmt.Errorf("fal.ai: unexpected queue status: %s", string(sBody))
		}
	}

fetchResult:
	rReq, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: queue result request error: %w", err)
	}
	rReq.Header.Set("Authorization", "Key "+f.apiKey)
	rReq.Header.Set("Accept", "application/json")

	rResp, err := pollClient.Do(rReq)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: queue result fetch failed: %w", err)
	}
	defer rResp.Body.Close()

	rBody, err := io.ReadAll(rResp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal.ai: queue result read error: %w", err)
	}

	if rResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fal.ai: queue result error (status %d): %s", rResp.StatusCode, string(rBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rBody, &result); err != nil {
		return nil, fmt.Errorf("fal.ai: queue result unmarshal error: %w", err)
	}

	resultURL = extractResultURL(result)

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

// klingV26ProAllowedParams lists params accepted by the Kling v2.6 Pro image-to-video model.
var klingV26ProAllowedParams = map[string]bool{
	"duration":        true,
	"negative_prompt": true,
	"generate_audio":  true,
	"voice_ids":        true,
	"end_image_url":   true,
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
		if input.ImageURL != "" {
			payload["reference_image_url"] = input.ImageURL
		}
	case isImageURLsModel(model):
		payload["image_urls"] = imageURLs
	case isKlingV26Model(model):
		if input.ImageURL != "" {
			payload["start_image_url"] = input.ImageURL
		}
		if input.NegativePrompt != "" {
			payload["negative_prompt"] = input.NegativePrompt
		}
	case isKlingVideoModel(model):
		if input.ImageURL != "" {
			payload["image_url"] = input.ImageURL
		}
	default:
		if input.ImageURL != "" {
			payload["image_url"] = input.ImageURL
		}
	}

	if input.MomImageURL != "" {
		payload["mom_image_url"] = input.MomImageURL
	}
	if input.BabyImageURL != "" {
		payload["baby_image_url"] = input.BabyImageURL
	}
	if input.DadImageURL != "" {
		payload["dad_image_url"] = input.DadImageURL
	}

	for k, v := range input.Params {
		if k == "aspect_ratio" {
			applyAspectRatio(payload, model, fmt.Sprintf("%v", v))
			continue
		}
		if k == "negative_prompt" && input.NegativePrompt != "" {
			continue
		}
		if isKlingV26Model(model) {
			if !klingV26ProAllowedParams[k] {
				continue
			}
			if k == "duration" {
				payload[k] = fmt.Sprintf("%v", v)
				continue
			}
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

func isKlingV26Model(model string) bool {
	return strings.Contains(model, "kling-video/v2") || strings.Contains(model, "kling-video/v3")
}

func isImageURLsModel(model string) bool {
	return strings.Contains(model, "nano-banana") || strings.Contains(model, "gemini-image") || strings.Contains(model, "seedream")
}

func isNativeAspectRatioModel(model string) bool {
	return strings.Contains(model, "nano-banana") || strings.Contains(model, "gemini-image")
}

func isQueueModel(model string) bool {
	return isKlingVideoModel(model)
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
