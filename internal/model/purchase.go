package model

import "time"

// Purchase tracks RevenueCat purchases that have been credited to a user.
// It prevents double-granting credits from both webhooks and sync-purchases.
type Purchase struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	FirebaseUID  string    `json:"firebase_uid" gorm:"index:idx_purchase_uid;not null"`
	ProductID    string    `json:"product_id" gorm:"index;not null"`
	RevenueCatID string    `json:"revenuecat_id" gorm:"uniqueIndex;not null"`
	Store        string    `json:"store" gorm:"index"`
	PurchasedAt  time.Time `json:"purchased_at"`
	Credits      int       `json:"credits"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
