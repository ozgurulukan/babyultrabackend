package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	AdminEmail         string
	FirebaseConfigPath string
	InitialCredits     int

	FalAIKey      string
	ReplicateKey  string
	DeepSeekKey   string
	OpenRouterKey string
	GeminiKey     string

	RevenueCatAPIKey        string
	RevenueCatProjectID     string
	RevenueCatWebhookSecret string

	FirebaseWebAPIKey   string
	FirebaseAuthDomain  string
	FirebaseProjectID   string
	FirebaseAppID       string

	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3BucketName      string
	S3PublicURL       string

	RateLimitMax    int
	RateLimitWindow int // seconds

	CORSAllowOrigins string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("WARN: .env file not found, using system environment variables")
	}

	return &Config{
		Port:               getEnv("PORT", "3000"),
		AdminEmail:         getEnv("ADMIN_EMAIL", ""),
		FirebaseConfigPath: getEnv("FIREBASE_CONFIG_PATH", "./firebase-service-account.json"),
		InitialCredits:     getEnvInt("INITIAL_CREDITS", 2),

		FalAIKey:      getEnv("FAL_AI_KEY", ""),
		ReplicateKey:  getEnv("REPLICATE_KEY", ""),
		DeepSeekKey:   getEnv("DEEPSEEK_KEY", ""),
		OpenRouterKey: getEnv("OPENROUTER_KEY", ""),
		GeminiKey:     getEnv("GEMINI_KEY", ""),

		RevenueCatAPIKey:        getEnv("REVENUECAT_API_KEY", ""),
		RevenueCatProjectID:     getEnv("REVENUECAT_PROJECT_ID", ""),
		RevenueCatWebhookSecret: getEnv("REVENUECAT_WEBHOOK_SECRET", ""),

		FirebaseWebAPIKey:   getEnv("FIREBASE_WEB_API_KEY", ""),
		FirebaseAuthDomain:  getEnv("FIREBASE_AUTH_DOMAIN", ""),
		FirebaseProjectID:   getEnv("FIREBASE_PROJECT_ID", ""),
		FirebaseAppID:       getEnv("FIREBASE_APP_ID", ""),

		S3Endpoint:        getEnv("S3_ENDPOINT", ""),
		S3AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
		S3SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
		S3Region:          getEnv("S3_REGION", "auto"),
		S3BucketName:      getEnv("S3_BUCKET_NAME", ""),
		S3PublicURL:       getEnv("S3_PUBLIC_URL", ""),

		RateLimitMax:    getEnvInt("RATE_LIMIT_MAX", 30),
		RateLimitWindow: getEnvInt("RATE_LIMIT_WINDOW", 60),

		CORSAllowOrigins: getEnv("CORS_ALLOW_ORIGINS", "*"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}
