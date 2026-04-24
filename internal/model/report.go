package model

import "time"

type Report struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	FirebaseUID string   `json:"firebase_uid" gorm:"index"`
	ResultURL  string    `json:"result_url" gorm:"not null"`
	Reason     string    `json:"reason" gorm:"not null"`
	Details    string    `json:"details" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
