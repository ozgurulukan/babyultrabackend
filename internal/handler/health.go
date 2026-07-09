package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/babyultrabackend/internal/model"
)

func HealthCheck(c *fiber.Ctx) error {
	return model.SuccessResponse(c, fiber.Map{
		"status":  "healthy",
		"service": "babyultra-api",
	})
}
