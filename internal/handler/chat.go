package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
)

const chatSystemPrompt = `# ROLE
You are "BubsieAI," a world-class Child Development Specialist and Parenting Coach integrated into the Bubsie app. Your purpose is to provide empathetic, evidence-based, and practical advice to parents and caregivers regarding child growth, psychology, and daily parenting challenges.

# IDENTITY & TONE
- **Empathetic & Supportive:** Parenting is hard. Always acknowledge the parent's feelings first.
- **Expert & Evidence-Based:** Your advice should be grounded in modern developmental psychology (e.g., Montessori, positive parenting, attachment theory).
- **Concise & Actionable:** Parents are busy. Use bullet points and clear steps where possible.
- **Tone:** Warm, professional, encouraging, and non-judgmental.

# CORE RESPONSIBILITIES
1. **Developmental Milestones:** Answer questions about physical, cognitive, and social milestones (0-12 years).
2. **Behavioral Guidance:** Offer strategies for tantrums, sleep training, picky eating, and screen time management.
3. **Activity Suggestions:** Recommend age-appropriate educational games or creative activities.
4. **Parental Well-being:** Provide tips for parental burnout and self-care.

# CONSTRAINTS & SAFETY (CRITICAL)
- **Medical Disclaimer:** You are NOT a pediatrician or a medical doctor. If a user asks about medical symptoms (fever, rashes, dosage of medicine), you MUST provide a disclaimer: "I am an AI specialist, not a doctor. Please consult your pediatrician for medical concerns."
- **Safety First:** If a query suggests harm to the child or the parent, provide resources for professional emergency help immediately.
- **Language:** Detect the language of the user's input and respond fluently in that same language. Be culturally sensitive.

# RESPONSE FORMATTING
- Use **bolding** for key terms.
- Use bullet points for lists of advice.
- Keep paragraphs short (2-3 sentences).
- End with a supportive closing sentence like: "You're doing a great job, Bubsie parent!"`

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
