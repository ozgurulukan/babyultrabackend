package handler

import (
	"fmt"
	"strings"
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

// RevenueCatWebhook handles RevenueCat V1/V2 webhooks.
// Configure this endpoint in RevenueCat Dashboard > Webhooks.
func (h *WebhookHandler) RevenueCatWebhook(c *fiber.Ctx) error {
	// Validate webhook secret
	authHeader := c.Get("Authorization", "")
	expected := "Bearer " + h.cfg.RevenueCatWebhookSecret
	if !strings.EqualFold(authHeader, expected) {
		fmt.Printf("[Webhook] Unauthorized: invalid authorization header\n")
		return model.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}

	var payload struct {
		Event struct {
			Type          string                 `json:"type"`
			AppUserID     string                 `json:"app_user_id"`
			ProductID     string                 `json:"product_id"`
			TransactionID string                 `json:"transaction_id"`
			Store         string                 `json:"store"`
			Entitlements  map[string]interface{} `json:"entitlements"`
		} `json:"event"`
	}

	body := c.Body()
	fmt.Printf("[Webhook] Received body: %s\n", string(body))

	if err := c.BodyParser(&payload); err != nil {
		fmt.Printf("[Webhook] BodyParser error: %v\n", err)
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid payload")
	}

	uid := payload.Event.AppUserID
	if uid == "" {
		fmt.Printf("[Webhook] Missing app_user_id\n")
		return model.ErrorResponse(c, fiber.StatusBadRequest, "missing app_user_id")
	}

	fmt.Printf("[Webhook] Event type=%s uid=%s product=%s entitlements=%d\n",
		payload.Event.Type, uid, payload.Event.ProductID, len(payload.Event.Entitlements))

	db := database.GetDB()

	// Update pro status based on entitlements for all event types
	hasProEntitlement := len(payload.Event.Entitlements) > 0
	if hasProEntitlement {
		fmt.Printf("[Webhook] Setting is_pro=true for uid=%s\n", uid)
		if err := db.Model(&model.User{}).
			Where("firebase_uid = ?", uid).
			Updates(map[string]interface{}{
				"is_pro":     true,
				"updated_at": time.Now(),
			}).Error; err != nil {
			fmt.Printf("[Webhook] Failed to update pro status: %v\n", err)
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
		}
	} else if payload.Event.Type == "EXPIRATION" || payload.Event.Type == "CANCELLATION" {
		fmt.Printf("[Webhook] Setting is_pro=false for uid=%s (event=%s)\n", uid, payload.Event.Type)
		if err := db.Model(&model.User{}).
			Where("firebase_uid = ?", uid).
			Updates(map[string]interface{}{
				"is_pro":     false,
				"updated_at": time.Now(),
			}).Error; err != nil {
			fmt.Printf("[Webhook] Failed to update pro status: %v\n", err)
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to update pro status")
		}
	}

	// Add credits only for one-time (non-renewing) purchase events
	switch payload.Event.Type {
	case "INITIAL_PURCHASE", "NON_RENEWING_PURCHASE":
		// One-time credit packs
		creditsToAdd := creditsForProduct(payload.Event.ProductID)
		if creditsToAdd > 0 {
			// Idempotency: use transaction_id as unique key; fallback to a deterministic synthetic ID.
			txID := payload.Event.TransactionID
			if txID == "" {
				txID = fmt.Sprintf("webhook-%s-%s-%d", uid, payload.Event.ProductID, time.Now().Unix())
			}
			var existing model.Purchase
			if db.Where("revenue_cat_id = ?", txID).First(&existing).Error == nil {
				fmt.Printf("[Webhook] Purchase %s already processed, skipping credit grant\n", txID)
			} else {
				fmt.Printf("[Webhook] Adding %d credits for uid=%s product=%s tx=%s\n", creditsToAdd, uid, payload.Event.ProductID, txID)
				if err := db.Create(&model.Purchase{
					FirebaseUID:  uid,
					ProductID:    payload.Event.ProductID,
					RevenueCatID: txID,
					Store:        payload.Event.Store,
					PurchasedAt:  time.Now(),
					Credits:      creditsToAdd,
				}).Error; err != nil {
					fmt.Printf("[Webhook] Failed to record purchase %s: %v\n", txID, err)
				} else {
					if err := db.Model(&model.User{}).
						Where("firebase_uid = ?", uid).
						Update("credits", gorm.Expr("credits + ?", creditsToAdd)).Error; err != nil {
						fmt.Printf("[Webhook] Failed to add credits: %v\n", err)
						return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to add credits")
					}
				}
			}
		}

		// Subscription initial grant: 50 weekly credits on first purchase
		if isSubscriptionProduct(payload.Event.ProductID) {
			now := time.Now().UTC()
			weekStart := now.AddDate(0, 0, -int(now.Weekday()-time.Monday))
			weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

			fmt.Printf("[Webhook] Adding weekly credits for uid=%s\n", uid)
			if err := db.Model(&model.User{}).
				Where("firebase_uid = ? AND (last_weekly_credit_at IS NULL OR last_weekly_credit_at < ?)", uid, weekStart).
				Updates(map[string]interface{}{
					"credits":               gorm.Expr("credits + ?", 50),
					"last_weekly_credit_at": now,
					"updated_at":            now,
				}).Error; err != nil {
				fmt.Printf("[Webhook] Failed to add weekly credits: %v\n", err)
				return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to add weekly credits")
			}
		}
	}

	fmt.Printf("[Webhook] Completed successfully for uid=%s\n", uid)
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

func isSubscriptionProduct(productID string) bool {
	switch productID {
	case "com.fagore.bubsie.weeklypro7", "com.fagore.bubsie.yearlypro7":
		return true
	default:
		return false
	}
}
