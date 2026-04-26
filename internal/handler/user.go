package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service"
	"github.com/ozgurulukan/bubsiebackend/internal/service/provider"
	"github.com/ozgurulukan/bubsiebackend/internal/service/storage"
	"gorm.io/gorm"
)

type UserHandler struct {
	cfg        *config.Config
	registry   *provider.Registry
	storage    *storage.R2Storage
	revenuecat *service.RevenueCatService
}

func NewUserHandler(cfg *config.Config, registry *provider.Registry, st *storage.R2Storage, rc *service.RevenueCatService) *UserHandler {
	return &UserHandler{cfg: cfg, registry: registry, storage: st, revenuecat: rc}
}

// POST /api/v1/sync-purchases
func (h *UserHandler) SyncPurchases(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	if uid == "" {
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	if h.revenuecat == nil {
		return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "revenuecat not configured")
	}

	info, err := h.revenuecat.GetCustomerInfo(c.Context(), uid)
	if err != nil {
		fmt.Printf("[SyncPurchases] GetCustomerInfo error for uid=%s: %v\n", uid, err)
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to fetch purchase info: "+err.Error())
	}

	fmt.Printf("[SyncPurchases] uid=%s isPro=%v\n", uid, info.Entitlements.Pro.IsActive)

	db := database.GetDB()

	// Update pro status based on entitlement
	isPro := info.Entitlements.Pro.IsActive
	if err := db.Model(&model.User{}).
		Where("firebase_uid = ?", uid).
		Update("is_pro", isPro).Error; err != nil {
		fmt.Printf("[SyncPurchases] Failed to update pro status for uid=%s: %v\n", uid, err)
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
	}

	// NOTE: Weekly Pro credits (50/week) are handled by credit_scheduler.go on Mondays.
	// Do NOT grant weekly credits here to avoid double-granting when users buy credit packs.

	// Sync one-time (credit pack) purchases from RevenueCat V2 purchases endpoint.
	purchases, err := h.revenuecat.GetCustomerPurchases(c.Context(), uid)
	if err != nil {
		fmt.Printf("[SyncPurchases] GetCustomerPurchases error for uid=%s: %v\n", uid, err)
		// Don't fail the whole sync if purchases can't be fetched; pro status was already updated.
	} else {
		for _, p := range purchases {
			creditsToAdd := creditsForProduct(p.ProductID)
			if creditsToAdd <= 0 {
				continue
			}
			// Idempotency: skip if already processed.
			var existing model.Purchase
			if db.Where("revenuecat_id = ?", p.ID).First(&existing).Error == nil {
				fmt.Printf("[SyncPurchases] Purchase %s already processed, skipping\n", p.ID)
				continue
			}
			// Record the purchase before adding credits (transaction-like safety).
			if err := db.Create(&model.Purchase{
				FirebaseUID:  uid,
				ProductID:    p.ProductID,
				RevenueCatID: p.ID,
				Store:        p.Store,
				PurchasedAt:  time.UnixMilli(p.PurchasedAt),
				Credits:      creditsToAdd,
			}).Error; err != nil {
				fmt.Printf("[SyncPurchases] Failed to record purchase %s for uid=%s: %v\n", p.ID, uid, err)
				continue
			}
			fmt.Printf("[SyncPurchases] Adding %d credits for uid=%s product=%s purchase=%s\n", creditsToAdd, uid, p.ProductID, p.ID)
			if err := db.Model(&model.User{}).
				Where("firebase_uid = ?", uid).
				Update("credits", gorm.Expr("credits + ?", creditsToAdd)).Error; err != nil {
				fmt.Printf("[SyncPurchases] Failed to add credits for uid=%s: %v\n", uid, err)
			}
		}
	}

	return model.SuccessResponse(c, fiber.Map{
		"is_pro": isPro,
	})
}

// POST /api/v1/me/pro
func (h *UserHandler) ActivatePro(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	if uid == "" {
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	db := database.GetDB()
	if err := db.Model(&model.User{}).
		Where("firebase_uid = ?", uid).
		Updates(map[string]interface{}{
			"is_pro":     true,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
	}

	return model.SuccessResponse(c, fiber.Map{
		"is_pro": true,
	})
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
		"uid":                    uid,
		"email":                  email,
		"name":                   user.Name,
		"photo":                  user.PhotoURL,
		"credits":                user.Credits,
		"revenuecat_customer_id": user.RevenueCatCustomerID,
		"is_pro":                 user.IsPro,
		"is_banned":              user.IsBanned,
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

// DELETE /api/v1/history/:id
func (h *UserHandler) DeleteHistoryItem(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	idParam := strings.TrimSpace(c.Params("id"))
	id, err := strconv.Atoi(idParam)
	if uid == "" || err != nil || id <= 0 {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid history item")
	}

	db := database.GetDB()
	result := db.Where("id = ? AND firebase_uid = ?", id, uid).Delete(&model.RequestLog{})
	if result.Error != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to delete history item")
	}
	if result.RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "history item not found")
	}

	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// POST /api/v1/me/delete
func (h *UserHandler) DeleteAccount(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	if uid == "" {
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	db := database.GetDB()

	// Check if a pending request already exists
	var existing model.DeletionRequest
	res := db.Where("firebase_uid = ? AND status = ?", uid, "pending").First(&existing)
	if res.Error == nil {
		return model.SuccessResponse(c, fiber.Map{
			"requested":      true,
			"requested_at":   existing.RequestedAt.Format(time.RFC3339),
			"message":        "Deletion request already pending",
		})
	}

	now := time.Now()

	// Mark user as deletion requested
	if err := db.Model(&model.User{}).
		Where("firebase_uid = ?", uid).
		Update("deletion_requested_at", now).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to request deletion")
	}

	// Create deletion request record for admin review
	if err := db.Create(&model.DeletionRequest{
		FirebaseUID: uid,
		Status:      "pending",
		RequestedAt: now,
	}).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create deletion request")
	}

	return model.SuccessResponse(c, fiber.Map{
		"requested":    true,
		"requested_at": now.Format(time.RFC3339),
		"message":      "Your deletion request has been submitted for review. You can continue using the app.",
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
