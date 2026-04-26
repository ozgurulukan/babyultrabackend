package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var SupportedLanguages = []string{
	"en", "tr", "de", "fr", "es", "pt", "it",
	"ja", "ko", "zh", "ar", "ru", "hi", "nl", "sv", "pl",
}

var LanguageNames = map[string]string{
	"en": "English", "tr": "Türkçe", "de": "Deutsch", "fr": "Français",
	"es": "Español", "pt": "Português", "it": "Italiano", "ja": "日本語",
	"ko": "한국어", "zh": "中文", "ar": "العربية", "ru": "Русский",
	"hi": "हिन्दी", "nl": "Nederlands", "sv": "Svenska", "pl": "Polski",
}

type TranslateService struct {
	apiKey string
	client *http.Client
	ready  bool
}

func NewTranslateService(deepseekKey string) *TranslateService {
	ts := &TranslateService{
		apiKey: deepseekKey,
		client: &http.Client{Timeout: 120 * time.Second},
	}
	if deepseekKey != "" && len(deepseekKey) > 8 {
		ts.ready = true
		log.Println("Translation service initialized (DeepSeek)")
	} else {
		log.Println("WARN: Translation service not configured (no DEEPSEEK_KEY)")
	}
	return ts
}

func (t *TranslateService) IsReady() bool {
	return t.ready
}

type TranslateResult struct {
	Translations map[string]string `json:"translations"`
}

func (t *TranslateService) TranslateToAll(ctx context.Context, text, sourceLang string, targetLangs []string) (map[string]string, error) {
	if !t.ready {
		return nil, fmt.Errorf("translation service not configured")
	}
	if text == "" {
		return map[string]string{}, nil
	}

	langList := ""
	for _, lang := range targetLangs {
		if name, ok := LanguageNames[lang]; ok {
			langList += fmt.Sprintf("- %s (%s)\n", lang, name)
		}
	}

	systemPrompt := `You are a professional translator for mobile app content. 
Translate the given text to ALL requested languages accurately. 
Keep the tone natural and appropriate for mobile app UI.
Do NOT translate brand names, technical terms, or proper nouns.
Return ONLY a valid JSON object with language codes as keys and translated texts as values.
Example: {"en": "Hello", "de": "Hallo", "fr": "Bonjour"}`

	userPrompt := fmt.Sprintf(`Translate this text from %s to all the following languages:

%s
Text to translate: "%s"

Return ONLY the JSON object, no markdown, no explanation.`, sourceLang, langList, text)

	payload := map[string]interface{}{
		"model": "deepseek-v4-flash",
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.3,
		"max_tokens":  4096,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("translate: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.deepseek.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("translate: request error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("translate: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("translate: read error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("translate: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("translate: unmarshal error: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("translate: empty response")
	}

	content := chatResp.Choices[0].Message.Content
	content = cleanJSONResponse(content)

	translations := make(map[string]string)
	if err := json.Unmarshal([]byte(content), &translations); err != nil {
		return nil, fmt.Errorf("translate: parse translations error: %w\nRaw: %s", err, content)
	}

	return translations, nil
}

func cleanJSONResponse(s string) string {
	start := -1
	end := -1
	depth := 0
	for i, ch := range s {
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if start >= 0 && end > start {
		return s[start:end]
	}
	return s
}
