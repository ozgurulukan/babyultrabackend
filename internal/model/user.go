package model

import "time"

type User struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	FirebaseUID        string     `json:"firebase_uid" gorm:"uniqueIndex;not null"`
	Email              string     `json:"email" gorm:"index"`
	Name               string     `json:"name"`
	PhotoURL           string     `json:"photo_url"`
	DeviceID           string     `json:"device_id" gorm:"index"`
	Credits            int        `json:"credits" gorm:"default:5"`
	IsPro              bool       `json:"is_pro" gorm:"default:false"`
	IsBanned           bool       `json:"is_banned" gorm:"default:false;index"`
	BanReason          string     `json:"ban_reason"`
	DeletedAt          *time.Time `json:"deleted_at" gorm:"index"`
	DeletionRequestedAt *time.Time `json:"deletion_requested_at" gorm:"index"`
	LastLogin          time.Time  `json:"last_login"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type DeletionRequest struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	FirebaseUID string     `json:"firebase_uid" gorm:"index;not null"`
	Status      string     `json:"status" gorm:"default:pending"` // pending, approved, rejected
	RequestedAt time.Time  `json:"requested_at"`
	ProcessedAt *time.Time `json:"processed_at"`
	ProcessedBy string     `json:"processed_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type RequestLog struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     uint      `json:"user_id" gorm:"index"`
	FirebaseUID string   `json:"firebase_uid" gorm:"index"`
	Provider   string    `json:"provider" gorm:"index"`
	Model      string    `json:"model"`
	Prompt     string    `json:"prompt"`
	ImageURL   string    `json:"image_url"`
	ResultURL  string    `json:"result_url"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

type ProviderSetting struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	Provider  string `json:"provider" gorm:"uniqueIndex;not null"`
	APIKey    string `json:"api_key"`
	IsActive  bool   `json:"is_active" gorm:"default:true"`
	Priority  int    `json:"priority" gorm:"default:0"`
	Models    []string `json:"models" gorm:"type:text;serializer:json"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Category struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	AppID       string    `json:"app_id" gorm:"index;not null;default:default"`
	Type        string    `json:"type" gorm:"index;not null;default:photo"` // photo | video
	IsPopular   bool      `json:"is_popular" gorm:"default:false;index"`
	IsTrending  bool      `json:"is_trending" gorm:"default:false;index"`
	Slug        string    `json:"slug" gorm:"not null"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	IconURL     string    `json:"icon_url"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	IsActive    bool      `json:"is_active" gorm:"default:true;index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Template struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	AppID           string    `json:"app_id" gorm:"index;not null;default:default"`
	Slug            string    `json:"slug" gorm:"uniqueIndex;not null"`
	Name            string    `json:"name" gorm:"not null"`
	Description     string    `json:"description"`
	ActionType      string    `json:"action_type" gorm:"index;default:image_generation"`
	Prompt          string    `json:"prompt" gorm:"type:text"`
	NegativePrompt  string    `json:"negative_prompt"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	CategoryID      uint      `json:"category_id" gorm:"index"`
	BeforeMediaURL  string    `json:"before_media_url"`
	BeforeMediaType string    `json:"before_media_type" gorm:"default:image"`
	AfterMediaURL   string    `json:"after_media_url"`
	AfterMediaType  string    `json:"after_media_type" gorm:"default:image"`
	ReferenceImageCount int   `json:"reference_image_count" gorm:"default:1"`
	ReferenceVideoURL   string `json:"reference_video_url"`
	RequireMomPhoto  bool `json:"require_mom_photo" gorm:"default:false"`
	RequireBabyPhoto bool `json:"require_baby_photo" gorm:"default:false"`
	RequireDadPhoto  bool `json:"require_dad_photo" gorm:"default:false"`
	HideFromAll           bool      `json:"hide_from_all" gorm:"default:false;index"`
	AspectRatio           string    `json:"aspect_ratio" gorm:"default:1:1"`
	SupportedAspectRatios string    `json:"supported_aspect_ratios" gorm:"type:text;default:1:1,4:5,9:16,16:9,3:4,4:3"`
	IconURL               string    `json:"icon_url"`
	Params                string    `json:"params" gorm:"type:text"`
	CreditCost      int       `json:"credit_cost" gorm:"default:1"`
	IsFree          bool      `json:"is_free" gorm:"default:false"`
	IsActive        bool      `json:"is_active" gorm:"default:true;index"`
	IsFeatured      bool      `json:"is_featured" gorm:"default:false;index"`
	IsPopular       bool      `json:"is_popular" gorm:"default:false;index"`
	IsViral         bool      `json:"is_viral" gorm:"default:false;index"`
	IsPremium       bool      `json:"is_premium" gorm:"default:false"`
	SortOrder       int       `json:"sort_order" gorm:"default:0"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SliderItem struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	AppID        string     `json:"app_id" gorm:"index;not null;default:default"`
	Type         string     `json:"type" gorm:"index;not null;default:photo"` // photo | video
	TemplateID   uint       `json:"template_id" gorm:"index"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	ImageURL     string     `json:"image_url"`
	FrameURL     string     `json:"frame_url"`
	DeepLink     string     `json:"deep_link"`
	SortOrder    int        `json:"sort_order" gorm:"default:0"`
	IsActive     bool       `json:"is_active" gorm:"default:true;index"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// QuickButton represents a shortcut button shown below the slider on the app home.
// Titles are intended to be global/English (no translation).
type QuickButton struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	AppID     string    `json:"app_id" gorm:"index;not null;default:default"`
	Type      string    `json:"type" gorm:"index;not null;default:photo"` // photo | video
	Title     string    `json:"title" gorm:"not null"`
	IconURL   string    `json:"icon_url"`
	TemplateID uint     `json:"template_id" gorm:"index"`
	SortOrder int       `json:"sort_order" gorm:"default:0"`
	IsActive  bool      `json:"is_active" gorm:"default:true;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OnboardingMedia struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	AppID       string    `json:"app_id" gorm:"index;not null;default:default"`
	Type        string    `json:"type" gorm:"not null"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	MediaURL    string    `json:"media_url" gorm:"not null"`
	ThumbnailURL string  `json:"thumbnail_url"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	IsActive    bool      `json:"is_active" gorm:"default:true;index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type OnboardingReview struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Nickname  string    `json:"nickname" gorm:"not null"`
	PhotoURL  string    `json:"photo_url"`
	Review    string    `json:"review" gorm:"type:text;not null"`
	Rating    int       `json:"rating" gorm:"default:5"`
	SortOrder int       `json:"sort_order" gorm:"default:0"`
	IsActive  bool      `json:"is_active" gorm:"default:true;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeviceToken represents a push-notification device token registered by a mobile app.
// One user (FirebaseUID) can own multiple tokens (e.g. iPhone + iPad).
type DeviceToken struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	FirebaseUID string    `json:"firebase_uid" gorm:"index;not null"`
	Token       string    `json:"token" gorm:"uniqueIndex;not null"`
	Platform    string    `json:"platform" gorm:"index;not null;default:ios"` // ios | android
	AppID       string    `json:"app_id" gorm:"index;not null;default:default"`
	Locale      string    `json:"locale"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type InstallCreditClaim struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	InstallSeedHash string    `json:"install_seed_hash" gorm:"uniqueIndex;not null"`
	FirstFirebaseUID string   `json:"first_firebase_uid" gorm:"index;not null"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Translation struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	EntityType string    `json:"entity_type" gorm:"index;not null"`
	EntityID   uint      `json:"entity_id" gorm:"index;not null"`
	Field      string    `json:"field" gorm:"not null"`
	Language   string    `json:"language" gorm:"index;not null"`
	Value      string    `json:"value" gorm:"type:text;not null"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type DeviceBan struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	DeviceID  string    `json:"device_id" gorm:"uniqueIndex;not null"`
	Reason    string    `json:"reason"`
	BannedBy  string    `json:"banned_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
