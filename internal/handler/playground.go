package handler

import (
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/babyultrabackend/internal/database"
	"github.com/ozgurulukan/babyultrabackend/internal/model"
	"github.com/ozgurulukan/babyultrabackend/internal/service/provider"
	"github.com/ozgurulukan/babyultrabackend/internal/service/storage"
)

type PlaygroundHandler struct {
	registry *provider.Registry
	storage  *storage.R2Storage
}

func NewPlaygroundHandler(registry *provider.Registry, st *storage.R2Storage) *PlaygroundHandler {
	return &PlaygroundHandler{registry: registry, storage: st}
}

func (h *PlaygroundHandler) TestTransform(c *fiber.Ctx) error {
	var req model.PlaygroundRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Provider == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "provider zorunlu")
	}
	if req.Prompt == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "prompt zorunlu")
	}

	if req.ImageURL != "" {
		parsedURL, err := url.Parse(req.ImageURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return model.ErrorResponse(c, fiber.StatusBadRequest, "image_url must be a valid HTTP(S) URL")
		}
	}


	p, err := h.registry.Get(req.Provider)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "unsupported or unconfigured provider")
	}

	start := time.Now()

	input := &provider.TransformInput{
		Model:           req.Model,
		ImageURL:        req.ImageURL,

		MomImageURL:     req.MomImageURL,
		BabyImageURL:    req.BabyImageURL,
		DadImageURL:     req.DadImageURL,
		Prompt:          req.Prompt,
		NegativePrompt:  req.NegativePrompt,
		Params:          req.Params,
	}

	result, err := p.Transform(c.Context(), input)
	duration := time.Since(start).Milliseconds()

	uid, _ := c.Locals("uid").(string)

	status := "success"
	resultURL := ""
	if err != nil {
		status = "error"
	} else if result != nil {
		resultURL = result.ResultURL

		if h.storage.IsReady() && resultURL != "" {
			folder := "playground/" + uid
			permanentURL, r2err := h.storage.UploadFromURL(c.Context(), resultURL, folder)
			if r2err != nil {
				log.Printf("R2 upload failed (playground) [uid=%s]: %v", uid, r2err)
			} else {
				result.ResultURL = permanentURL
				resultURL = permanentURL
			}
		}
	}

	go logPlaygroundRequest(uid, req.Provider, req.Model, req.ActionType, req.Prompt, req.ImageURL, resultURL, status, duration)

	if err != nil {
		log.Printf("Playground error [provider=%s, uid=%s]: %v", req.Provider, uid, err)
		return model.ErrorResponse(c, fiber.StatusBadGateway, err.Error())
	}

	return model.SuccessResponse(c, fiber.Map{
		"result":      result,
		"duration_ms": duration,
		"provider":    req.Provider,
		"model":       req.Model,
		"action_type": req.ActionType,
	})
}

func logPlaygroundRequest(uid, providerName, modelName, actionType, prompt, imageURL, resultURL, status string, durationMs int64) {
	db := database.GetDB()
	if db == nil {
		return
	}

	var user model.User
	if err := db.Where("firebase_uid = ?", uid).First(&user).Error; err == nil {
		_ = user.ID
	}

	db.Create(&model.RequestLog{
		FirebaseUID: uid,
		Provider:    "playground:" + providerName,
		Model:       modelName,
		Prompt:      prompt,
		ImageURL:    imageURL,
		ResultURL:   resultURL,
		Status:      status,
		DurationMs:  durationMs,
	})

	_ = actionType
}

func (h *PlaygroundHandler) PlaygroundMeta(c *fiber.Ctx) error {
	actionTypes := []fiber.Map{
		{"value": "text_to_text", "label": "Text → Text"},
		{"value": "text_to_image", "label": "Text → Image"},
		{"value": "text_to_audio", "label": "Text → Audio"},
		{"value": "image_to_image", "label": "Image → Image"},
		{"value": "image_to_text", "label": "Image → Text"},

		{"value": "audio_to_text", "label": "Audio → Text"},
	}

	providers := h.registry.List()

	return model.SuccessResponse(c, fiber.Map{
		"action_types": actionTypes,
		"providers":    providers,
	})
}

func sanitizeFolder(folder string) string {
	folder = strings.ReplaceAll(folder, "..", "")
	folder = strings.ReplaceAll(folder, "/", "")
	folder = strings.ReplaceAll(folder, "\\", "")
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = "media"
	}
	return folder
}

func isSafeFolder(folder string) bool {
	return !strings.Contains(folder, "..") && !strings.Contains(folder, "/") && !strings.Contains(folder, "\\")
}