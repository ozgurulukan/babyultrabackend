package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
)

type WebhookHandler struct {
	cfg *config.Config
}

func NewWebhookHandler(cfg *config.Config) *WebhookHandler {
	return &WebhookHandler{cfg: cfg}
}

// RevenueCatWebhook handles RevenueCat V2 webhooks.
// Configure this endpoint in RevenueCat Dashboard > Webhooks.
func (h *WebhookHandler) RevenueCatWebhook(c *fiber.Ctx) error {
	var payload struct {
		Event struct {
			Type           string   `json:"type"`
			AppUserID      string   `json:"app_user_id"`
			ProductID      string   `json:"product_id"`
			EntitlementIDs []string `json:"entitlement_ids"`
		} `json:"event"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid payload")
	}

	uid := payload.Event.AppUserID
	if uid == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "missing app_user_id")
	}

	db := database.GetDB()

	switch payload.Event.Type {
	case "INITIAL_PURCHASE", "RENEWAL", "NON_RENEWING_PURCHASE", "PRODUCT_CHANGE":
		if len(payload.Event.EntitlementIDs) > 0 {
			if err := db.Model(&model.User{}).
				Where("firebase_uid = ?", uid).
				Updates(map[string]interface{}{
					"is_pro":     true,
					"updated_at": time.Now(),
				}).Error; err != nil {
				return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
			}
		}
	case "EXPIRATION", "CANCELLATION":
		// Only set is_pro=false if there are no remaining entitlements
		if len(payload.Event.EntitlementIDs) == 0 {
			if err := db.Model(&model.User{}).
				Where("firebase_uid = ?", uid).
				Updates(map[string]interface{}{
					"is_pro":     false,
					"updated_at": time.Now(),
				}).Error; err != nil {
				return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
