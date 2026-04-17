package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service/provider"
	"github.com/ozgurulukan/bubsiebackend/internal/service/storage"
)

type UserHandler struct {
	cfg      *config.Config
	registry *provider.Registry
	storage  *storage.R2Storage
}

func NewUserHandler(cfg *config.Config, registry *provider.Registry, st *storage.R2Storage) *UserHandler {
	return &UserHandler{cfg: cfg, registry: registry, storage: st}
}

// GET /api/v1/me
func (h *UserHandler) GetProfile(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	email, _ := c.Locals("email").(string)

	db := database.GetDB()

	var user model.User
	db.Where("firebase_uid = ?", uid).First(&user)

	today := time.Now().UTC().Truncate(24 * time.Hour)

	var todayUsage int64
	db.Model(&model.RequestLog{}).
		Where("firebase_uid = ? AND created_at >= ?", uid, today).
		Count(&todayUsage)

	var totalUsage int64
	db.Model(&model.RequestLog{}).
		Where("firebase_uid = ?", uid).
		Count(&totalUsage)

	var todaySuccess int64
	db.Model(&model.RequestLog{}).
		Where("firebase_uid = ? AND created_at >= ? AND status = ?", uid, today, "success").
		Count(&todaySuccess)

	return model.SuccessResponse(c, fiber.Map{
		"uid":       uid,
		"email":     email,
		"name":      user.Name,
		"photo":     user.PhotoURL,
		"credits":   user.Credits,
		"is_pro":    user.IsPro,
		"is_banned": user.IsBanned,
		"usage": fiber.Map{
			"today_total":   todayUsage,
			"today_success": todaySuccess,
			"all_time":      totalUsage,
		},
		"rate_limit": fiber.Map{
			"max_per_window": h.cfg.RateLimitMax,
			"window_seconds": h.cfg.RateLimitWindow,
		},
		"member_since": user.CreatedAt,
	})
}

// GET /api/v1/providers
func (h *UserHandler) GetProviders(c *fiber.Ctx) error {
	db := database.GetDB()

	var settings []model.ProviderSetting
	db.Find(&settings)
	settingsMap := make(map[string]*model.ProviderSetting)
	for i := range settings {
		settingsMap[settings[i].Provider] = &settings[i]
	}

	type publicProvider struct {
		Name    string `json:"name"`
		Active  bool   `json:"active"`
	}

	available := h.registry.List()
	availableMap := make(map[string]bool)
	for _, name := range available {
		availableMap[name] = true
	}

	allNames := []string{"fal.ai", "replicate", "deepseek", "openrouter", "gemini"}
	providers := make([]publicProvider, 0)
	for _, name := range allNames {
		hasKey := availableMap[name]
		enabled := true
		if s, ok := settingsMap[name]; ok {
			enabled = s.IsActive
		}
		providers = append(providers, publicProvider{
			Name:   name,
			Active: hasKey && enabled,
		})
	}

	return model.SuccessResponse(c, fiber.Map{
		"providers": providers,
	})
}

// GET /api/v1/history
func (h *UserHandler) GetHistory(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if limit > 50 {
		limit = 50
	}
	offset := (page - 1) * limit

	db := database.GetDB()

	var total int64
	db.Model(&model.RequestLog{}).Where("firebase_uid = ?", uid).Count(&total)

	var logs []model.RequestLog
	db.Where("firebase_uid = ?", uid).
		Order("created_at desc").
		Offset(offset).
		Limit(limit).
		Find(&logs)

	type historyItem struct {
		ID         uint   `json:"id"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		Prompt     string `json:"prompt"`
		ImageURL   string `json:"image_url"`
		ResultURL  string `json:"result_url"`
		Status     string `json:"status"`
		DurationMs int64  `json:"duration_ms"`
		CreatedAt  string `json:"created_at"`
	}

	items := make([]historyItem, 0, len(logs))
	for _, l := range logs {
		items = append(items, historyItem{
			ID:         l.ID,
			Provider:   l.Provider,
			Model:      l.Model,
			Prompt:     l.Prompt,
			ImageURL:   l.ImageURL,
			ResultURL:  l.ResultURL,
			Status:     l.Status,
			DurationMs: l.DurationMs,
			CreatedAt:  l.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	return model.SuccessResponse(c, fiber.Map{
		"history": items,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// POST /api/v1/upload
func (h *UserHandler) UploadImage(c *fiber.Ctx) error {
	file, err := c.FormFile("image")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "image file is required")
	}

	if file.Size > 10*1024*1024 {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "image must be under 10MB")
	}

	ext := filepath.Ext(file.Filename)
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".heic": true}
	if !allowed[ext] {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "allowed formats: jpg, jpeg, png, webp, heic")
	}

	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	filename := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102_150405"), hex.EncodeToString(randomBytes), ext)

	uid, _ := c.Locals("uid").(string)

	// R2 upload
	if h.storage.IsReady() {
		src, err := file.Open()
		if err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to read image")
		}
		defer src.Close()

		data, err := io.ReadAll(src)
		if err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to read image data")
		}

		contentType := http.DetectContentType(data)
		key := fmt.Sprintf("uploads/%s/%s", uid, filename)

		imageURL, err := h.storage.Upload(c.Context(), key, data, contentType)
		if err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to upload image: "+err.Error())
		}

		return model.SuccessResponse(c, fiber.Map{
			"url":      imageURL,
			"filename": filename,
			"size":     file.Size,
		})
	}

	// Fallback: local storage
	uploadDir := "data/uploads"
	os.MkdirAll(uploadDir, 0755)

	savePath := filepath.Join(uploadDir, filename)
	if err := c.SaveFile(file, savePath); err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to save image")
	}

	scheme := "https"
	host := c.Hostname()
	imageURL := fmt.Sprintf("%s://%s/uploads/%s", scheme, host, filename)

	return model.SuccessResponse(c, fiber.Map{
		"url":      imageURL,
		"filename": filename,
		"size":     file.Size,
	})
}
