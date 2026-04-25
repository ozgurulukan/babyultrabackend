package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"gorm.io/gorm"
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

	// Update pro status based on entitlements for all event types
	if len(payload.Event.EntitlementIDs) > 0 {
		if err := db.Model(&model.User{}).
			Where("firebase_uid = ?", uid).
			Updates(map[string]interface{}{
				"is_pro":     true,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
		}
	} else if payload.Event.Type == "EXPIRATION" || payload.Event.Type == "CANCELLATION" {
		if err := db.Model(&model.User{}).
			Where("firebase_uid = ?", uid).
			Updates(map[string]interface{}{
				"is_pro":     false,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
		}
	}

	// Add credits for consumable / one-time purchases
	creditsToAdd := creditsForProduct(payload.Event.ProductID)
	if creditsToAdd > 0 {
		if err := db.Model(&model.User{}).
			Where("firebase_uid = ?", uid).
			Update("credits", gorm.Expr("credits + ?", creditsToAdd)).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to add credits")
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func creditsForProduct(productID string) int {
	switch productID {
	case "com.fagore.bubsie.100credits":
		return 100
	case "com.fagore.bubsie.250credits":
		return 250
	case "com.fagore.bubsie.1000credits":
		return 1000
	default:
		return 0
	}
}
