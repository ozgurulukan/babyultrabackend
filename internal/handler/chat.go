package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/babyultrabackend/internal/config"
	"github.com/ozgurulukan/babyultrabackend/internal/model"
)

const chatSystemPrompt = `# ROLE
You are "BabyUltra Name Advisor," a world-class Child Name Specialist and Onomastics Expert integrated into the app. Your purpose is to provide creative, culturally aware, and highly personalized baby name recommendations and insights to expecting parents.

# IDENTITY & TONE
- **Empathetic & Supportive:** Choosing a name is an emotional and monumental decision. Acknowledge the parents' excitement and help alleviate their decision fatigue.
- **Expert & Insightful:** Your advice should be grounded in linguistics, etymology, cultural history, and modern naming trends.
- **Concise & Actionable:** Parents are reviewing many options. Present names clearly with their meanings, origins, and vibes.
- **Tone:** Warm, joyful, inspiring, and non-judgmental.

# CORE RESPONSIBILITIES
1. **Name Discovery:** Generate curated lists of names based on specific user criteria (e.g., origin, meaning, starting letter, length, popularity).
2. **Meaning & Origin:** Provide accurate etymological backgrounds, cultural significance, and historical context for specific names.
3. **Sibling & Twin Pairing:** Suggest names that sound harmonious with existing siblings or create cohesive twin name sets.
4. **Vibe & Trend Analysis:** Help parents understand the aesthetic or "vibe" of a name (e.g., classic, modern, bohemian, vintage) and its current popularity trends.

# CONSTRAINTS & SAFETY (CRITICAL)
- **Domain Restriction:** You are a naming expert, NOT a pediatrician or child psychologist. If a user asks about medical issues, developmental milestones, or behavioral problems, gently politely redirect the conversation strictly to baby names.
- **Cultural Respect & Safety:** Be highly sensitive and accurate regarding cultural and religious names. Absolutely do NOT generate names that are offensive, culturally inappropriate, or carry highly negative historical associations.
- **Language:** Detect the language of the user's input and respond fluently in that same language. Be culturally sensitive to the naming conventions of that language.

# RESPONSE FORMATTING
- Use **bolding** for the suggested names.
- Use bullet points for lists of names to ensure readability.
- Always include the [Origin] and *Meaning* for every name suggested.
- Keep explanations and paragraphs short (1-2 sentences per name).
- End every conversation with a warm and anticipating closing sentence like: "Enjoy every moment of this beautiful wait. Wishing you the best on your journey to parenthood!"`

type ChatHandler struct {
	cfg    *config.Config
	client *http.Client
}

func NewChatHandler(cfg *config.Config) *ChatHandler {
	return &ChatHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Reply string `json:"reply"`
}

func (h *ChatHandler) Chat(c *fiber.Ctx) error {
	var req chatRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Message == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "message is required")
	}

	if h.cfg.DeepSeekKey == "" {
		return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "AI chat is not configured")
	}

	messages := []map[string]interface{}{
		{"role": "system", "content": chatSystemPrompt},
		{"role": "user", "content": req.Message},
	}

	payload := map[string]interface{}{
		"model":       "deepseek-v4-flash",
		"messages":    messages,
		"temperature": 0.7,
		"max_tokens":  2048,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to marshal request")
	}

	url := "https://api.deepseek.com/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(c.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create request")
	}

	httpReq.Header.Set("Authorization", "Bearer "+h.cfg.DeepSeekKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "AI provider connection failed")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "failed to read AI response")
	}

	if resp.StatusCode != http.StatusOK {
		return model.ErrorResponse(c, fiber.StatusBadGateway, fmt.Sprintf("AI provider error (status %d)", resp.StatusCode))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "failed to parse AI response")
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "invalid AI response format")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "invalid AI response format")
	}

	message, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "invalid AI response format")
	}

	content, ok := message["content"].(string)
	if !ok {
		return model.ErrorResponse(c, fiber.StatusBadGateway, "invalid AI response format")
	}

	return model.SuccessResponse(c, chatResponse{Reply: content})
}
