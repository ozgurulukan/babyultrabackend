package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/babyultrabackend/internal/database"
	"github.com/ozgurulukan/babyultrabackend/internal/model"
)

type ReportHandler struct{}

func NewReportHandler() *ReportHandler {
	return &ReportHandler{}
}

// POST /api/v1/reports — mobile users submit a report
func (h *ReportHandler) CreateReport(c *fiber.Ctx) error {
	var req model.CreateReportRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.ResultURL == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "result_url is required")
	}
	if req.Reason == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "reason is required")
	}

	uid, _ := c.Locals("uid").(string)

	db := database.GetDB()
	report := model.Report{
		FirebaseUID: uid,
		ResultURL:   req.ResultURL,
		Reason:      req.Reason,
		Details:     req.Details,
	}
	if err := db.Create(&report).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create report: "+err.Error())
	}

	return model.SuccessResponse(c, fiber.Map{"id": report.ID})
}

// GET /api/admin/reports — admin list all reports
func (h *ReportHandler) AdminListReports(c *fiber.Ctx) error {
	db := database.GetDB()
	var reports []model.Report
	db.Order("created_at desc").Find(&reports)
	return model.SuccessResponse(c, fiber.Map{"reports": reports})
}

// DELETE /api/admin/reports/:id — admin delete a report
func (h *ReportHandler) AdminDeleteReport(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	if db.Delete(&model.Report{}, id).RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "report not found")
	}
	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// Helper used by admin panel to also optionally delete the content from R2 / server
func (h *ReportHandler) AdminDeleteReportAndContent(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	var report model.Report
	if err := db.First(&report, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "report not found")
	}

	// Delete report record
	db.Delete(&report)

	// Return the result_url so the admin panel can also call R2 deletion if needed
	return model.SuccessResponse(c, fiber.Map{
		"deleted":    true,
		"result_url": report.ResultURL,
	})
}

func slugifyReportReason(r string) string {
	s := strings.ToLower(strings.TrimSpace(r))
	s = strings.ReplaceAll(s, " ", "_")
	var clean []rune
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			clean = append(clean, c)
		}
	}
	return string(clean)
}
