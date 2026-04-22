package handler

import (
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service"
)

type NotificationHandler struct {
	firebase *service.FirebaseService
}

func NewNotificationHandler(firebase *service.FirebaseService) *NotificationHandler {
	return &NotificationHandler{firebase: firebase}
}

// ─── Mobile API ─────────────────────────────────────────────

// POST /api/v1/device-token
// Body: { "token": "fcm_token_str", "platform": "ios", "app_id": "default", "locale": "en" }
// Upserts a device token for the authenticated user.
func (h *NotificationHandler) RegisterDeviceToken(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	if uid == "" {
		log.Printf("[DeviceToken] Unauthorized: no uid in context")
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
		AppID    string `json:"app_id"`
		Locale   string `json:"locale"`
	}
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "token is required")
	}

	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform != "ios" && platform != "android" {
		platform = "ios"
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		appID = "default"
	}

	log.Printf("[DeviceToken] Registering token for uid=%s platform=%s app_id=%s", uid, platform, appID)

	db := database.GetDB()

	var existing model.DeviceToken
	res := db.Where("token = ?", req.Token).First(&existing)
	if res.Error == nil {
		db.Model(&existing).Updates(map[string]interface{}{
			"firebase_uid": uid,
			"platform":     platform,
			"app_id":       appID,
			"locale":       req.Locale,
			"updated_at":   time.Now(),
		})
		return model.SuccessResponse(c, fiber.Map{
			"id":       existing.ID,
			"registered": true,
		})
	}

	tok := model.DeviceToken{
		FirebaseUID: uid,
		Token:       req.Token,
		Platform:    platform,
		AppID:       appID,
		Locale:      req.Locale,
	}
	if err := db.Create(&tok).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to save token")
	}

	return model.SuccessResponse(c, fiber.Map{
		"id":         tok.ID,
		"registered": true,
	})
}

// DELETE /api/v1/device-token
// Body: { "token": "fcm_token_str" }
// Removes the given device token for the authenticated user (e.g. on logout).
func (h *NotificationHandler) DeleteDeviceToken(c *fiber.Ctx) error {
	uid, _ := c.Locals("uid").(string)
	if uid == "" {
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "token is required")
	}

	db := database.GetDB()
	db.Where("token = ? AND firebase_uid = ?", req.Token, uid).Delete(&model.DeviceToken{})

	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// ─── Admin API ──────────────────────────────────────────────

// GET /api/admin/notifications/stats
// Returns the number of registered device tokens broken down by platform.
func (h *NotificationHandler) AdminTokenStats(c *fiber.Ctx) error {
	db := database.GetDB()

	type row struct {
		Platform string
		Count    int64
	}

	var rows []row
	db.Model(&model.DeviceToken{}).
		Select("platform, count(*) as count").
		Group("platform").
		Scan(&rows)

	stats := fiber.Map{
		"ios":     int64(0),
		"android": int64(0),
		"total":   int64(0),
	}
	var total int64
	for _, r := range rows {
		stats[r.Platform] = r.Count
		total += r.Count
	}
	stats["total"] = total
	stats["messaging_ready"] = h.firebase.IsMessagingReady()

	return model.SuccessResponse(c, stats)
}

// POST /api/admin/notifications/send
// Body:
// {
//   "title": "Yeni template!",
//   "body":  "Birlikte deneyelim",
//   "platform": "ios",             // ios | android | all (default: all)
//   "app_id":   "default",         // optional, filters by app_id
//   "deep_link": "luris://home",   // optional, added to data payload
//   "target_uids": ["uid1"],       // optional, if provided only these UIDs receive it
//   "data": { "key": "value" }     // optional extra data payload
// }
func (h *NotificationHandler) AdminSendNotification(c *fiber.Ctx) error {
	if !h.firebase.IsMessagingReady() {
		return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "firebase messaging is not configured on the server")
	}

	var req struct {
		Title      string            `json:"title"`
		Body       string            `json:"body"`
		Platform   string            `json:"platform"`
		AppID      string            `json:"app_id"`
		DeepLink   string            `json:"deep_link"`
		TargetUIDs []string          `json:"target_uids"`
		Data       map[string]string `json:"data"`
	}
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.Title == "" || req.Body == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "title and body are required")
	}

	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		platform = "all"
	}
	if platform != "ios" && platform != "android" && platform != "all" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "platform must be ios, android or all")
	}

	db := database.GetDB()
	query := db.Model(&model.DeviceToken{})
	if platform != "all" {
		query = query.Where("platform = ?", platform)
	}
	if appID := strings.TrimSpace(req.AppID); appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	if len(req.TargetUIDs) > 0 {
		query = query.Where("firebase_uid IN ?", req.TargetUIDs)
	}

	var tokens []string
	query.Distinct("token").Pluck("token", &tokens)

	if len(tokens) == 0 {
		return model.SuccessResponse(c, fiber.Map{
			"sent":          0,
			"failed":        0,
			"total_targets": 0,
			"message":       "no device tokens matched the target filter",
		})
	}

	data := map[string]string{}
	for k, v := range req.Data {
		data[k] = v
	}
	if req.DeepLink != "" {
		data["deep_link"] = req.DeepLink
	}

	success, failure, invalid, err := h.firebase.SendMulticast(c.Context(), tokens, req.Title, req.Body, data)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to send notification: "+err.Error())
	}

	if len(invalid) > 0 {
		db.Where("token IN ?", invalid).Delete(&model.DeviceToken{})
	}

	return model.SuccessResponse(c, fiber.Map{
		"sent":            success,
		"failed":          failure,
		"total_targets":   len(tokens),
		"pruned_invalid":  len(invalid),
	})
}
