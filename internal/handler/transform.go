package handler

import (
	"errors"
	"log"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service/provider"
	"github.com/ozgurulukan/bubsiebackend/internal/service/storage"
	"gorm.io/gorm"
)

type TransformHandler struct {
	registry *provider.Registry
	storage  *storage.R2Storage
}

var errInsufficientCredits = errors.New("insufficient credits")

func NewTransformHandler(registry *provider.Registry, st *storage.R2Storage) *TransformHandler {
	return &TransformHandler{registry: registry, storage: st}
}

func (h *TransformHandler) Transform(c *fiber.Ctx) error {
	var req model.TransformRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Provider == "" || req.Prompt == "" || (req.ImageURL == "" && len(req.ImageURLs) == 0) {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "missing required fields")
	}

	// Normalize multi-image input
	if len(req.ImageURLs) > 0 && req.ImageURL == "" {
		req.ImageURL = req.ImageURLs[0]
	}
	if req.ImageURL != "" && len(req.ImageURLs) == 0 {
		req.ImageURLs = []string{req.ImageURL}
	}

	if req.ImageURL != "" {
		parsedURL, err := url.Parse(req.ImageURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return model.ErrorResponse(c, fiber.StatusBadRequest, "image_url must be a valid HTTP(S) URL")
		}
	}
	for _, u := range req.ImageURLs {
		parsedURL, err := url.Parse(u)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return model.ErrorResponse(c, fiber.StatusBadRequest, "image_urls must be valid HTTP(S) URLs")
		}
	}

	p, err := h.registry.Get(req.Provider)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "unsupported or unconfigured provider")
	}

	uid, _ := c.Locals("uid").(string)
	if uid == "" {
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	transformCreditCost := req.CreditCost
	if transformCreditCost <= 0 {
		transformCreditCost = 1
	}
	creditReserved, err := reserveTransformCredit(uid, transformCreditCost)
	if err != nil {
		if errors.Is(err, errInsufficientCredits) {
			return model.ErrorResponse(c, fiber.StatusPaymentRequired, "insufficient credits")
		}
		log.Printf("Credit reserve failed [uid=%s]: %v", uid, err)
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to reserve credit")
	}

	start := time.Now()

	input := &provider.TransformInput{
		Model:           req.Model,
		ImageURL:        req.ImageURL,
		ImageURLs:       req.ImageURLs,
		MomImageURL:     req.MomImageURL,
		BabyImageURL:    req.BabyImageURL,
		DadImageURL:     req.DadImageURL,
		Prompt:          req.Prompt,
		NegativePrompt:  req.NegativePrompt,
		Params:          req.Params,
	}

	logID := createRequestLog(uid, req.Provider, req.Model, req.Prompt, req.ImageURL, "", "processing", 0)

	result, err := p.Transform(c.Context(), input)
	duration := time.Since(start).Milliseconds()

	status := "success"
	resultURL := ""
	if err != nil {
		if creditReserved {
			refundTransformCredit(uid, transformCreditCost)
		}
		status = "error"
	} else if result != nil {
		resultURL = result.ResultURL

		if h.storage.IsReady() && resultURL != "" {
			folder := "results/" + uid
			permanentURL, r2err := h.storage.UploadFromURL(c.Context(), resultURL, folder)
			if r2err != nil {
				log.Printf("R2 upload failed (keeping original URL) [uid=%s]: %v", uid, r2err)
			} else {
				result.ResultURL = permanentURL
				resultURL = permanentURL
			}
		}
	}

	if logID > 0 {
		updateRequestLog(logID, status, resultURL, duration)
	}

	if err != nil {
		log.Printf("Transform error [provider=%s, uid=%s]: %v", req.Provider, uid, err)
		return model.ErrorResponse(c, fiber.StatusBadGateway, "AI provider error — please try again")
	}

	return model.SuccessResponse(c, result)
}

func createRequestLog(uid, providerName, modelName, prompt, imageURL, resultURL, status string, durationMs int64) uint {
	db := database.GetDB()
	if db == nil {
		return 0
	}

	var user model.User
	var userID uint
	if err := db.Where("firebase_uid = ?", uid).First(&user).Error; err == nil {
		userID = user.ID
	}

	logEntry := &model.RequestLog{
		UserID:      userID,
		FirebaseUID: uid,
		Provider:    providerName,
		Model:       modelName,
		Prompt:      prompt,
		ImageURL:    imageURL,
		ResultURL:   resultURL,
		Status:      status,
		DurationMs:  durationMs,
	}
	if err := db.Create(logEntry).Error; err != nil {
		log.Printf("Failed to create request log [uid=%s]: %v", uid, err)
		return 0
	}
	return logEntry.ID
}

func updateRequestLog(id uint, status, resultURL string, durationMs int64) {
	db := database.GetDB()
	if db == nil {
		return
	}
	if err := db.Model(&model.RequestLog{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"result_url": resultURL,
		"duration_ms": durationMs,
	}).Error; err != nil {
		log.Printf("Failed to update request log [id=%d]: %v", id, err)
	}
}

func reserveTransformCredit(uid string, cost int) (bool, error) {
	db := database.GetDB()
	if db == nil {
		return false, errors.New("database is not ready")
	}

	var user model.User
	if err := db.Select("id", "is_pro", "credits").Where("firebase_uid = ?", uid).First(&user).Error; err != nil {
		return false, err
	}

	if user.IsPro || cost <= 0 {
		return false, nil
	}

	res := db.Model(&model.User{}).
		Where("id = ? AND is_pro = ? AND credits >= ?", user.ID, false, cost).
		Update("credits", gorm.Expr("credits - ?", cost))
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, errInsufficientCredits
	}

	return true, nil
}

func refundTransformCredit(uid string, cost int) {
	if cost <= 0 {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}
	if err := db.Model(&model.User{}).
		Where("firebase_uid = ? AND is_pro = ?", uid, false).
		Update("credits", gorm.Expr("credits + ?", cost)).Error; err != nil {
		log.Printf("Credit refund failed [uid=%s]: %v", uid, err)
	}
}
