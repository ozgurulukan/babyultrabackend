package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/handler"
	"github.com/ozgurulukan/bubsiebackend/internal/middleware"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service"
	"github.com/ozgurulukan/bubsiebackend/internal/service/provider"
	"github.com/ozgurulukan/bubsiebackend/internal/service/storage"
	"github.com/ozgurulukan/bubsiebackend/internal/web"
)

func Setup(
	app *fiber.App,
	cfg *config.Config,
	firebase *service.FirebaseService,
	registry *provider.Registry,
	revenuecat *service.RevenueCatService,
	r2 *storage.R2Storage,
	translator *service.TranslateService,
) {
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Install-Seed",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		return c.Next()
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/panel")
	})

	app.Get("/health", handler.HealthCheck)

	app.Get("/api/config/firebase", func(c *fiber.Ctx) error {
		if cfg.FirebaseWebAPIKey == "" {
			return model.ErrorResponse(c, fiber.StatusNotFound, "firebase web config not set")
		}
		return model.SuccessResponse(c, fiber.Map{
			"apiKey":     cfg.FirebaseWebAPIKey,
			"authDomain": cfg.FirebaseAuthDomain,
			"projectId":  cfg.FirebaseProjectID,
			"appId":      cfg.FirebaseAppID,
		})
	})

	// Serve uploaded images
	app.Static("/uploads", "./data/uploads", fiber.Static{
		Browse: false,
	})

	web.RegisterRoutes(app)

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitMax, cfg.RateLimitWindow)
	adminRateLimiter := middleware.NewRateLimiter(120, 60)

	// Strict per-endpoint limiters for expensive operations
	transformLimiter := middleware.NewRateLimiter(3, 60)
	chatLimiter := middleware.NewRateLimiter(10, 60)
	uploadLimiter := middleware.NewRateLimiter(5, 60)

	transformHandler := handler.NewTransformHandler(registry, r2, firebase)
	userHandler := handler.NewUserHandler(cfg, registry, r2, revenuecat)
	adminHandler := handler.NewAdminHandler(cfg, registry, firebase, revenuecat)
	playgroundHandler := handler.NewPlaygroundHandler(registry, r2)
	contentHandler := handler.NewContentHandler(r2, translator)
	notificationHandler := handler.NewNotificationHandler(firebase)
	chatHandler := handler.NewChatHandler(cfg)
	reportHandler := handler.NewReportHandler()

	api := app.Group("/api")

	// Mobile API
	v1 := api.Group("/v1")

	// Public webhook endpoint (no auth required)
	webhookHandler := handler.NewWebhookHandler(cfg)
	v1.Post("/webhooks/revenuecat", webhookHandler.RevenueCatWebhook)

	v1.Use(middleware.FirebaseAuth(firebase, cfg.InitialCredits))
	v1.Use(rateLimiter.Middleware())

	v1.Post("/transform", transformLimiter.Middleware(), transformHandler.Transform)
	v1.Post("/upload", uploadLimiter.Middleware(), userHandler.UploadImage)
	v1.Post("/chat", chatLimiter.Middleware(), chatHandler.Chat)
	v1.Post("/reports", reportHandler.CreateReport)

	v1.Get("/me", userHandler.GetProfile)
	v1.Post("/me/pro", userHandler.ActivatePro)
	v1.Post("/sync-purchases", userHandler.SyncPurchases)
	v1.Post("/me/delete", userHandler.DeleteAccount)
	v1.Get("/providers", userHandler.GetProviders)
	v1.Get("/history", userHandler.GetHistory)
	v1.Delete("/history/:id", userHandler.DeleteHistoryItem)
	v1.Post("/history/:id/delete", userHandler.DeleteHistoryItem)

	v1.Get("/categories", contentHandler.GetCategories)
	v1.Get("/templates", contentHandler.GetTemplates)
	v1.Get("/slider", contentHandler.GetSlider)
	v1.Get("/quick-buttons", contentHandler.GetQuickButtons)
	v1.Get("/onboarding", contentHandler.GetOnboarding)
	v1.Get("/reviews", contentHandler.GetReviews)
	v1.Get("/languages", contentHandler.GetLanguages)

	v1.Post("/device-token", notificationHandler.RegisterDeviceToken)
	v1.Delete("/device-token", notificationHandler.DeleteDeviceToken)

	// Admin API
	admin := api.Group("/admin")
	admin.Use(middleware.LightweightFirebaseAuth(cfg.FirebaseProjectID, cfg.AdminEmail))
	admin.Use(middleware.AdminOnly(cfg.AdminEmail))
	admin.Use(adminRateLimiter.Middleware())
	admin.Get("/stats", adminHandler.GetStats)
	admin.Get("/stats/revenue", adminHandler.GetRevenue)
	admin.Get("/stats/revenue-detailed", adminHandler.GetRevenueDetailed)
	admin.Get("/users/count", adminHandler.GetUserCount)
	admin.Get("/providers", adminHandler.ListProviders)
	admin.Get("/providers/health-check", adminHandler.HealthCheckProviders)
	admin.Post("/providers/test", adminHandler.TestProvider)
	admin.Post("/providers/toggle", adminHandler.ToggleProvider)
	admin.Post("/providers/update-keys", adminHandler.UpdateProviderKey)
	admin.Get("/logs", adminHandler.GetRequestLogs)
	admin.Delete("/logs", adminHandler.DeleteRequestLogs)
	admin.Get("/users", adminHandler.ListUsers)
	admin.Post("/users/sync-pro-status", adminHandler.SyncProStatus)
	admin.Put("/users/:id", adminHandler.UpdateUserCredits)
	admin.Delete("/users/:id", adminHandler.DeleteUser)

	admin.Get("/deletion-requests", adminHandler.ListDeletionRequests)
	admin.Post("/deletion-requests/:id/approve", adminHandler.ApproveDeletionRequest)
	admin.Post("/deletion-requests/:id/reject", adminHandler.RejectDeletionRequest)

	admin.Get("/device-bans", adminHandler.ListBannedDevices)
	admin.Post("/device-bans", adminHandler.BanDevice)
	admin.Delete("/device-bans/:id", adminHandler.UnbanDevice)

	admin.Get("/categories", contentHandler.AdminListCategories)
	admin.Post("/categories", contentHandler.AdminCreateCategory)
	admin.Put("/categories/:id", contentHandler.AdminUpdateCategory)
	admin.Delete("/categories/:id", contentHandler.AdminDeleteCategory)

	admin.Get("/templates", contentHandler.AdminListTemplates)
	admin.Post("/templates", contentHandler.AdminCreateTemplate)
	admin.Put("/templates/:id", contentHandler.AdminUpdateTemplate)
	admin.Delete("/templates/:id", contentHandler.AdminDeleteTemplate)
	admin.Post("/templates/reorder", contentHandler.AdminReorderTemplates)

	admin.Get("/slider", contentHandler.AdminListSlider)
	admin.Post("/slider", contentHandler.AdminCreateSlider)
	admin.Put("/slider/:id", contentHandler.AdminUpdateSlider)
	admin.Delete("/slider/:id", contentHandler.AdminDeleteSlider)
	admin.Post("/slider/reorder", contentHandler.AdminReorderSlider)

	admin.Get("/quick-buttons", contentHandler.AdminListQuickButtons)
	admin.Post("/quick-buttons", contentHandler.AdminCreateQuickButton)
	admin.Put("/quick-buttons/:id", contentHandler.AdminUpdateQuickButton)
	admin.Delete("/quick-buttons/:id", contentHandler.AdminDeleteQuickButton)

	admin.Get("/onboarding", contentHandler.AdminListOnboarding)
	admin.Post("/onboarding", contentHandler.AdminCreateOnboarding)
	admin.Put("/onboarding/:id", contentHandler.AdminUpdateOnboarding)
	admin.Delete("/onboarding/:id", contentHandler.AdminDeleteOnboarding)

	admin.Get("/reviews", contentHandler.AdminListReviews)
	admin.Post("/reviews", contentHandler.AdminCreateReview)
	admin.Put("/reviews/:id", contentHandler.AdminUpdateReview)
	admin.Delete("/reviews/:id", contentHandler.AdminDeleteReview)

	admin.Post("/translate", contentHandler.AdminTranslate)
	admin.Get("/translations", contentHandler.AdminGetTranslations)

	admin.Post("/upload-media", contentHandler.AdminUploadMedia)

	admin.Post("/playground", playgroundHandler.TestTransform)
	admin.Get("/playground/meta", playgroundHandler.PlaygroundMeta)

	admin.Get("/notes", contentHandler.AdminListNotes)
	admin.Post("/notes", contentHandler.AdminCreateNote)
	admin.Put("/notes/:id", contentHandler.AdminUpdateNote)
	admin.Delete("/notes/:id", contentHandler.AdminDeleteNote)

	admin.Get("/notifications/stats", notificationHandler.AdminTokenStats)
	admin.Post("/notifications/send", notificationHandler.AdminSendNotification)

	admin.Get("/reports", reportHandler.AdminListReports)
	admin.Delete("/reports/:id", reportHandler.AdminDeleteReport)

	admin.Get("/check", func(c *fiber.Ctx) error {
		return model.SuccessResponse(c, fiber.Map{
			"admin": true,
			"email": c.Locals("email"),
		})
	})
}
