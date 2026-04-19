package database

import (
	"log"
	"os"

	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect() {
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Printf("WARN: Failed to create data directory: %v", err)
	}

	var err error
	DB, err = gorm.Open(sqlite.Open("data/bubsiebackend.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := DB.AutoMigrate(
		&model.User{},
		&model.RequestLog{},
		&model.ProviderSetting{},
		&model.Category{},
		&model.Template{},
		&model.SliderItem{},
		&model.QuickButton{},
		&model.OnboardingMedia{},
		&model.OnboardingReview{},
		&model.Translation{},
		&model.DeviceToken{},
		&model.InstallCreditClaim{},
		&model.Note{},
	); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Category slug uniqueness should be scoped by (app_id, type).
	// Use raw SQL so SQLite never recreates the table (which would lose data).
	DB.Exec("DROP INDEX IF EXISTS idx_categories_slug")
	DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_categories_app_type_slug ON categories(app_id, type, slug)")

	log.Println("Database connected and migrated")
}

func GetDB() *gorm.DB {
	return DB
}
