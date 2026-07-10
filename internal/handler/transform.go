package handler

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/babyultrabackend/internal/database"
	"github.com/ozgurulukan/babyultrabackend/internal/model"
	"github.com/ozgurulukan/babyultrabackend/internal/service"
	"github.com/ozgurulukan/babyultrabackend/internal/service/provider"
	"github.com/ozgurulukan/babyultrabackend/internal/service/storage"
	"github.com/ozgurulukan/babyultrabackend/internal/util/imageutil"
	"gorm.io/gorm"
)

type TransformHandler struct {
	registry *provider.Registry
	storage  *storage.R2Storage
	firebase *service.FirebaseService
}

var errInsufficientCredits = errors.New("insufficient credits")

func NewTransformHandler(registry *provider.Registry, st *storage.R2Storage, firebase *service.FirebaseService) *TransformHandler {
	return &TransformHandler{registry: registry, storage: st, firebase: firebase}
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
	log.Printf("[Transform] uid=%s provider=%s creditCost=%d", uid, req.Provider, transformCreditCost)
	creditReserved, err := reserveTransformCredit(uid, transformCreditCost)
	if err != nil {
		if errors.Is(err, errInsufficientCredits) {
			return model.ErrorResponse(c, fiber.StatusPaymentRequired, "insufficient credits")
		}
		log.Printf("Credit reserve failed [uid=%s]: %v", uid, err)
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to reserve credit")
	}
	log.Printf("[Transform] creditReserved=%v for uid=%s", creditReserved, uid)

	// --- Resize images if needed (fal.ai max 3850x3850) ---
	const falMaxDim = 3840 // safety margin under 3850

	processImageURL := func(rawURL string) string {
		if rawURL == "" {
			return ""
		}
		// Skip already-processed URLs to avoid infinite loops
		if h.storage.IsReady() && strings.Contains(rawURL, "/processed/") {
			return rawURL
		}
		resizedImg, format, err := imageutil.ResizeImageFromURL(c.Context(), rawURL, falMaxDim)
		if err != nil {
			if !errors.Is(err, imageutil.ErrNoResizeNeeded) {
				log.Printf("Image resize check failed for url=%s: %v", rawURL, err)
			}
			return rawURL
		}
		if !h.storage.IsReady() {
			log.Printf("Storage not ready, cannot upload resized image")
			return rawURL
		}
		data, contentType, err := imageutil.EncodeImage(resizedImg, format, 90)
		if err != nil {
			log.Printf("Image encode failed: %v", err)
			return rawURL
		}
		randBytes := make([]byte, 8)
		if _, err := rand.Read(randBytes); err != nil {
			log.Printf("rand.Read failed: %v", err)
			return rawURL
		}
		ext := filepath.Ext(rawURL)
		if ext == "" || len(ext) > 5 {
			if contentType == "image/png" {
				ext = ".png"
			} else {
				ext = ".jpg"
			}
		}
		key := fmt.Sprintf("processed/%s/%s_%x%s", uid, time.Now().Format("20060102_150405"), randBytes, ext)
		newURL, err := h.storage.Upload(c.Context(), key, data, contentType)
		if err != nil {
			log.Printf("Resized image upload failed: %v", err)
			return rawURL
		}
		log.Printf("Resized image uploaded: %s -> %s", rawURL, newURL)
		return newURL
	}

	imageURL := processImageURL(req.ImageURL)
	var imageURLs []string
	for _, u := range req.ImageURLs {
		if nu := processImageURL(u); nu != "" {
			imageURLs = append(imageURLs, nu)
		}
	}
	momImageURL := processImageURL(req.MomImageURL)
	babyImageURL := processImageURL(req.BabyImageURL)
	dadImageURL := processImageURL(req.DadImageURL)
	// --------------------------------------------------------

	start := time.Now()

	input := &provider.TransformInput{
		Model:           req.Model,
		ImageURL:        imageURL,
		ImageURLs:       imageURLs,

		MomImageURL:     momImageURL,
		BabyImageURL:    babyImageURL,
		DadImageURL:     dadImageURL,
		Prompt:          req.Prompt,
		NegativePrompt:  req.NegativePrompt,
		Params:          req.Params,
	}

	logID := createRequestLog(uid, req.Provider, req.Model, req.Prompt, imageURL, "", "processing", 0)

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

	if req.NotifyWhenDone {
		go sendCompletionPush(h.firebase, uid, req.Prompt, resultURL)
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
		log.Printf("[reserveTransformCredit] user not found [uid=%s]: %v", uid, err)
		return false, err
	}

	log.Printf("[reserveTransformCredit] user found [uid=%s] isPro=%v credits=%d cost=%d", uid, user.IsPro, user.Credits, cost)

	if cost <= 0 {
		log.Printf("[reserveTransformCredit] skipping credit deduction [uid=%s] reason=cost<=0", uid)
		return false, nil
	}

	res := db.Model(&model.User{}).
		Where("id = ? AND credits >= ?", user.ID, cost).
		Update("credits", gorm.Expr("credits - ?", cost))
	if res.Error != nil {
		log.Printf("[reserveTransformCredit] update error [uid=%s]: %v", uid, res.Error)
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		log.Printf("[reserveTransformCredit] insufficient credits [uid=%s] rowsAffected=0", uid)
		return false, errInsufficientCredits
	}

	log.Printf("[reserveTransformCredit] credit deducted [uid=%s] cost=%d remaining=%d", uid, cost, user.Credits-cost)
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
		Where("firebase_uid = ?", uid).
		Update("credits", gorm.Expr("credits + ?", cost)).Error; err != nil {
		log.Printf("Credit refund failed [uid=%s]: %v", uid, err)
	}
}

func sendCompletionPush(fb *service.FirebaseService, uid, prompt, resultURL string) {
	if fb == nil || !fb.IsMessagingReady() {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}

	var tokens []string
	if err := db.Model(&model.DeviceToken{}).
		Where("firebase_uid = ?", uid).
		Distinct("token").
		Pluck("token", &tokens).Error; err != nil {
		log.Printf("Failed to fetch device tokens [uid=%s]: %v", uid, err)
		return
	}
	if len(tokens) == 0 {
		log.Printf("[Push] No device tokens found for uid=%s, skipping completion push", uid)
		return
	}

	title := "Your result is ready"
	body := "Your AI transformation has completed. Tap to view it."
	if prompt != "" {
		body = "Your transformation is ready. Tap to view it."
	}

	data := map[string]string{}
	if resultURL != "" {
		data["result_url"] = resultURL
	}

	_, _, invalid, err := fb.SendMulticast(context.Background(), tokens, title, body, data)
	if err != nil {
		log.Printf("Push notification failed [uid=%s]: %v", uid, err)
	}
	if len(invalid) > 0 {
		db.Where("token IN ?", invalid).Delete(&model.DeviceToken{})
	}
}
