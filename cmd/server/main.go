package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/babyultrabackend/internal/config"
	"github.com/ozgurulukan/babyultrabackend/internal/database"
	"github.com/ozgurulukan/babyultrabackend/internal/router"
	"github.com/ozgurulukan/babyultrabackend/internal/service"
	"github.com/ozgurulukan/babyultrabackend/internal/service/provider"
	"github.com/ozgurulukan/babyultrabackend/internal/service/storage"
)

func main() {
	log.Println("Starting BabyUltra Api...")

	cfg := config.Load()

	database.Connect()

	firebase := service.NewFirebaseService(cfg.FirebaseConfigPath)
	if !firebase.IsReady() {
		log.Println("WARN: Firebase is not configured — auth endpoints will return 503")
	}

	registry := provider.NewRegistry(cfg)
	log.Printf("Registered providers: %v", registry.List())

	revenuecat := service.NewRevenueCatService(cfg.RevenueCatAPIKey, cfg.RevenueCatProjectID)

	r2 := storage.NewR2Storage(
		cfg.S3Endpoint,
		cfg.S3AccessKeyID,
		cfg.S3SecretAccessKey,
		cfg.S3Region,
		cfg.S3BucketName,
		cfg.S3PublicURL,
	)

	app := fiber.New(fiber.Config{
		AppName:       "BabyUltra Api",
		ServerHeader:  "",
		BodyLimit:     50 * 1024 * 1024,
		ReadTimeout:   360 * time.Second,
		WriteTimeout:  360 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "internal server error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			}
			if code >= 500 {
				log.Printf("Unhandled error [%s %s]: %v", c.Method(), c.Path(), err)
				msg = "internal server error"
			}
			return c.Status(code).JSON(fiber.Map{
				"success": false,
				"error":   msg,
			})
		},
	})

	translator := service.NewTranslateService(cfg.DeepSeekKey)

	router.Setup(app, cfg, firebase, registry, revenuecat, r2, translator)

	service.StartWeeklyCreditScheduler(revenuecat)

	go func() {
		addr := ":" + cfg.Port
		log.Printf("Listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if err := app.Shutdown(); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped gracefully")
}
