package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
)

func AdminOnly(adminEmail string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		email, ok := c.Locals("email").(string)
		if !ok || email == "" {
			return model.ErrorResponse(c, fiber.StatusForbidden, "admin access required")
		}

		if email != adminEmail {
			return model.ErrorResponse(c, fiber.StatusForbidden, "you do not have admin privileges")
		}

		c.Locals("is_admin", true)
		return c.Next()
	}
}
