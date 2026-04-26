package service

import (
	"log"
	"time"

	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"gorm.io/gorm"
)

const weeklyProCredits = 50

// StartWeeklyCreditScheduler starts a background goroutine that runs every day
// at 00:01. On Mondays it grants 50 credits to all active Pro users.
func StartWeeklyCreditScheduler() {
	go func() {
		for {
			now := time.Now().UTC()
			// Calculate next 00:01
			next := time.Date(now.Year(), now.Month(), now.Day(), 0, 1, 0, 0, time.UTC)
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			sleep := time.Until(next)
			log.Printf("[Scheduler] Next daily check at %s (sleeping %v)", next.Format(time.RFC3339), sleep)
			time.Sleep(sleep)

			if time.Now().UTC().Weekday() == time.Monday {
				grantWeeklyCredits()
			}
		}
	}()
}

func grantWeeklyCredits() {
	db := database.GetDB()
	if db == nil {
		log.Println("[Scheduler] Database not available, skipping weekly credits")
		return
	}

	now := time.Now().UTC()
	// Start of this week (Monday 00:00 UTC)
	daysSinceMonday := int(now.Weekday() - time.Monday)
	if daysSinceMonday < 0 {
		daysSinceMonday += 7
	}
	weekStart := now.AddDate(0, 0, -daysSinceMonday)
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	var users []model.User
	if err := db.Where("is_pro = ? AND (last_weekly_credit_at IS NULL OR last_weekly_credit_at < ?)", true, weekStart).Find(&users).Error; err != nil {
		log.Printf("[Scheduler] Failed to fetch pro users: %v", err)
		return
	}

	if len(users) == 0 {
		log.Println("[Scheduler] No pro users need weekly credits this week")
		return
	}

	for _, u := range users {
		if err := db.Model(&model.User{}).
			Where("id = ?", u.ID).
			Updates(map[string]interface{}{
				"credits":                 gorm.Expr("credits + ?", weeklyProCredits),
				"last_weekly_credit_at":   now,
				"updated_at":              now,
			}).Error; err != nil {
			log.Printf("[Scheduler] Failed to grant weekly credits to user %s: %v", u.FirebaseUID, err)
		} else {
			log.Printf("[Scheduler] Granted %d weekly credits to user %s", weeklyProCredits, u.FirebaseUID)
		}
	}

	log.Printf("[Scheduler] Weekly credits processed for %d pro user(s)", len(users))
}
