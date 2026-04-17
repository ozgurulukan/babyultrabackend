package web

import (
	"embed"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

//go:embed templates/*
var templateFS embed.FS

func RegisterRoutes(app *fiber.App) {
	app.Use("/panel", filesystem.New(filesystem.Config{
		Root:       http.FS(templateFS),
		PathPrefix: "templates",
		Index:      "index.html",
		Browse:     false,
	}))
}
