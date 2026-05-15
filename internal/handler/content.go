package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service"
	"github.com/ozgurulukan/bubsiebackend/internal/service/storage"
	"gorm.io/gorm"
)

type ContentHandler struct {
	storage   *storage.R2Storage
	translate *service.TranslateService
}

func NewContentHandler(st *storage.R2Storage, ts *service.TranslateService) *ContentHandler {
	return &ContentHandler{storage: st, translate: ts}
}

// ─── Mobile API ─────────────────────────────────────────────

// GET /api/v1/categories?app_id=xxx&lang=en
func (h *ContentHandler) GetCategories(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "default")
	lang := c.Query("lang", "")
	catType := strings.TrimSpace(c.Query("type", ""))

	var categories []model.Category
	q := db.Where("is_active = ? AND app_id = ?", true, appID)
	if catType != "" {
		if catType == "photo" {
			// Backward compatible: treat empty/null as photo
			q = q.Where("(type = ? OR type IS NULL OR type = '')", "photo")
		} else {
			q = q.Where("type = ?", catType)
		}
	}
	q.
		Order("sort_order asc").Find(&categories)

	if lang != "" {
		h.applyTranslations(categories, "category", lang)
	}

	// App-level category tabs should always start with: All, Popular, Viral
	// These are virtual "categories" (id=0) and are always English.
	virtual := []fiber.Map{
		{"id": 0, "type": catType, "slug": "all", "name": "All", "description": "", "is_active": true, "sort_order": -4, "is_virtual": true, "filter": "all"},
		{"id": 0, "type": catType, "slug": "popular", "name": "Popular", "description": "", "is_active": true, "sort_order": -3, "is_virtual": true, "filter": "popular"},
		{"id": 0, "type": catType, "slug": "viral", "name": "Viral", "description": "", "is_active": true, "sort_order": -2, "is_virtual": true, "filter": "viral"},
		{"id": 0, "type": catType, "slug": "other", "name": "Other", "description": "", "is_active": true, "sort_order": -1, "is_virtual": true, "filter": "other"},
	}
	out := make([]interface{}, 0, len(virtual)+len(categories))
	for _, v := range virtual {
		// If type isn't provided, default to photo for virtual entries.
		if v["type"] == "" {
			v["type"] = "photo"
		}
		out = append(out, v)
	}
	for i := range categories {
		if categories[i].Type == "" {
			categories[i].Type = "photo"
		}
		out = append(out, categories[i])
	}

	return model.SuccessResponse(c, fiber.Map{
		"categories": out,
	})
}

// GET /api/v1/templates?app_id=xxx&category_id=1&lang=en
func (h *ContentHandler) GetTemplates(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "default")
	categoryID := c.QueryInt("category_id", 0)
	featured := c.Query("featured", "")
	popular := c.Query("popular", "")
	viral := c.Query("viral", "")
	if viral == "" {
		viral = c.Query("trending", "") // backward compatibility
	}
	catType := strings.TrimSpace(c.Query("type", ""))
	includeHidden := c.Query("include_hidden", "") == "true"
	lang := c.Query("lang", "")

	query := db.Model(&model.Template{}).Select("templates.*").Where("templates.is_active = ? AND templates.app_id = ?", true, appID)
	if categoryID > 0 {
		query = query.Where("templates.category_id = ?", categoryID)
	}
	if featured == "true" {
		query = query.Where("templates.is_featured = ?", true)
	}
	if popular == "true" {
		query = query.Where("templates.is_popular = ?", true)
	}
	if viral == "true" {
		query = query.Where("templates.is_viral = ?", true)
	}

	if !includeHidden && categoryID == 0 && featured != "true" && popular != "true" && viral != "true" {
		query = query.Where("templates.hide_from_all = ?", false)
	}

	needsJoin := catType != ""
	if needsJoin {
		query = query.Joins("LEFT JOIN categories ON categories.id = templates.category_id")

		if catType != "" {
			if catType == "photo" {
				query = query.Where("(categories.type = ? OR categories.type IS NULL OR categories.type = '')", "photo")
			} else {
				query = query.Where("categories.type = ?", catType)
			}
		}
	}

	var templates []model.Template
	query.Order("templates.sort_order asc, templates.created_at desc").Find(&templates)

	if lang != "" {
		h.applyTranslations(templates, "template", lang)
	}

	return model.SuccessResponse(c, fiber.Map{
		"templates": templates,
	})
}

// GET /api/v1/slider?app_id=xxx&lang=en
func (h *ContentHandler) GetSlider(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "default")
	lang := c.Query("lang", "")
	sliderType := strings.TrimSpace(c.Query("type", ""))
	now := time.Now()

	var items []model.SliderItem
	q := db.Where("is_active = ? AND (app_id = ? OR app_id = 'default') AND (starts_at IS NULL OR starts_at <= ?) AND (ends_at IS NULL OR ends_at >= ?)",
		true, appID, now, now).
		Order("sort_order asc")
	if sliderType != "" {
		if sliderType == "photo" {
			q = q.Where("(type = ? OR type IS NULL OR type = '')", "photo")
		} else {
			q = q.Where("type = ?", sliderType)
		}
	}
	q.Find(&items)

	if lang != "" {
		h.applyTranslations(items, "slider", lang)
	}

	return model.SuccessResponse(c, fiber.Map{
		"slider": items,
	})
}

// GET /api/v1/quick-buttons?app_id=xxx&type=photo
func (h *ContentHandler) GetQuickButtons(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "default")
	btnType := strings.TrimSpace(c.Query("type", "photo"))

	query := db.Model(&model.QuickButton{}).Where("is_active = ? AND app_id = ?", true, appID)
	if btnType != "" {
		if btnType == "photo" {
			query = query.Where("(type = ? OR type IS NULL OR type = '')", "photo")
		} else {
			query = query.Where("type = ?", btnType)
		}
	}

	var buttons []model.QuickButton
	query.Order("sort_order asc, updated_at desc").Find(&buttons)
	return model.SuccessResponse(c, fiber.Map{"buttons": buttons})
}

// GET /api/v1/onboarding?app_id=xxx&lang=en
func (h *ContentHandler) GetOnboarding(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "default")
	lang := c.Query("lang", "")

	var media []model.OnboardingMedia
	db.Where("is_active = ? AND app_id = ?", true, appID).
		Order("sort_order asc").
		Find(&media)

	if lang != "" {
		h.applyTranslations(media, "onboarding", lang)
	}

	return model.SuccessResponse(c, fiber.Map{
		"onboarding": media,
	})
}

// GET /api/v1/reviews
func (h *ContentHandler) GetReviews(c *fiber.Ctx) error {
	db := database.GetDB()
	lang := c.Query("lang", "")

	var reviews []model.OnboardingReview
	db.Where("is_active = ?", true).
		Order("sort_order asc").
		Find(&reviews)

	if lang != "" {
		h.applyTranslations(reviews, "review", lang)
	}

	return model.SuccessResponse(c, fiber.Map{
		"reviews": reviews,
	})
}

// GET /api/v1/languages
func (h *ContentHandler) GetLanguages(c *fiber.Ctx) error {
	langs := make([]fiber.Map, 0, len(service.SupportedLanguages))
	for _, code := range service.SupportedLanguages {
		langs = append(langs, fiber.Map{
			"code": code,
			"name": service.LanguageNames[code],
		})
	}
	return model.SuccessResponse(c, fiber.Map{
		"languages": langs,
	})
}

// ─── Admin API ──────────────────────────────────────────────

// ── Categories ──

// GET /api/admin/categories
func (h *ContentHandler) AdminListCategories(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "")
	catType := strings.TrimSpace(c.Query("type", ""))
	query := db.Model(&model.Category{})
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	if catType != "" {
		if catType == "photo" {
			// Backward compatible: treat empty/null as photo
			query = query.Where("(type = ? OR type IS NULL OR type = '')", "photo")
		} else {
			query = query.Where("type = ?", catType)
		}
	}
	var categories []model.Category
	query.Order("sort_order asc").Find(&categories)
	return model.SuccessResponse(c, fiber.Map{"categories": categories})
}

// POST /api/admin/categories
func (h *ContentHandler) AdminCreateCategory(c *fiber.Ctx) error {
	var cat model.Category
	if err := c.BodyParser(&cat); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if cat.Name == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "name is required")
	}
	if cat.Type == "" {
		cat.Type = "photo"
	}
	if cat.Slug == "" {
		cat.Slug = slugify(cat.Name)
	}
	if cat.AppID == "" {
		cat.AppID = "default"
	}
	db := database.GetDB()
	if err := db.Create(&cat).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return model.ErrorResponse(c, fiber.StatusConflict, "slug already exists")
		}
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create: "+err.Error())
	}
	return model.SuccessResponse(c, cat)
}

// PUT /api/admin/categories/:id
func (h *ContentHandler) AdminUpdateCategory(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	var existing model.Category
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "category not found")
	}
	var updates model.Category
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	db.Model(&existing).Updates(map[string]interface{}{
		"app_id": nonEmpty(updates.AppID, existing.AppID), "slug": nonEmpty(updates.Slug, existing.Slug),
		"name": nonEmpty(updates.Name, existing.Name), "description": updates.Description,
		"type": nonEmpty(updates.Type, existing.Type),
		"is_popular": updates.IsPopular,
		"is_trending": updates.IsTrending,
		"icon_url": updates.IconURL, "sort_order": updates.SortOrder, "is_active": updates.IsActive,
	})
	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

// DELETE /api/admin/categories/:id
func (h *ContentHandler) AdminDeleteCategory(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	if db.Delete(&model.Category{}, id).RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}
	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// ── Slider ──

// GET /api/admin/slider
func (h *ContentHandler) AdminListSlider(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "")
	sliderType := strings.TrimSpace(c.Query("type", ""))
	query := db.Model(&model.SliderItem{})
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	if sliderType != "" {
		if sliderType == "photo" {
			query = query.Where("(type = ? OR type IS NULL OR type = '')", "photo")
		} else {
			query = query.Where("type = ?", sliderType)
		}
	}
	var items []model.SliderItem
	query.Order("sort_order asc").Find(&items)
	return model.SuccessResponse(c, fiber.Map{"slider": items})
}

// POST /api/admin/slider
func (h *ContentHandler) AdminCreateSlider(c *fiber.Ctx) error {
	var item model.SliderItem
	if err := c.BodyParser(&item); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if item.AppID == "" {
		item.AppID = "default"
	}
	if item.Type == "" {
		item.Type = "photo"
	}
	db := database.GetDB()
	if err := db.Create(&item).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create: "+err.Error())
	}
	return model.SuccessResponse(c, item)
}

// PUT /api/admin/slider/:id
func (h *ContentHandler) AdminUpdateSlider(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	var existing model.SliderItem
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}
	var updates model.SliderItem
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	db.Model(&existing).Updates(map[string]interface{}{
		"app_id": nonEmpty(updates.AppID, existing.AppID), "template_id": updates.TemplateID,
		"type": nonEmpty(updates.Type, existing.Type),
		"title": updates.Title, "description": updates.Description,
		"image_url": updates.ImageURL, "frame_url": updates.FrameURL,
		"deep_link": updates.DeepLink, "sort_order": updates.SortOrder,
		"is_active": updates.IsActive, "starts_at": updates.StartsAt, "ends_at": updates.EndsAt,
	})
	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

// DELETE /api/admin/slider/:id
func (h *ContentHandler) AdminDeleteSlider(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	if db.Delete(&model.SliderItem{}, id).RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}
	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// POST /api/admin/slider/reorder
func (h *ContentHandler) AdminReorderSlider(c *fiber.Ctx) error {
	type reorderItem struct {
		ID        uint `json:"id"`
		SortOrder int  `json:"sort_order"`
	}
	var body struct {
		Items []reorderItem `json:"items"`
	}
	if err := c.BodyParser(&body); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	db := database.GetDB()
	for _, item := range body.Items {
		if item.ID == 0 {
			continue
		}
		db.Model(&model.SliderItem{}).Where("id = ?", item.ID).Update("sort_order", item.SortOrder)
	}
	return model.SuccessResponse(c, fiber.Map{"reordered": true})
}

// ── Quick Buttons ──

// GET /api/admin/quick-buttons
func (h *ContentHandler) AdminListQuickButtons(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "")
	btnType := strings.TrimSpace(c.Query("type", ""))

	query := db.Model(&model.QuickButton{})
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	if btnType != "" {
		if btnType == "photo" {
			query = query.Where("(type = ? OR type IS NULL OR type = '')", "photo")
		} else {
			query = query.Where("type = ?", btnType)
		}
	}

	var items []model.QuickButton
	query.Order("sort_order asc, updated_at desc").Find(&items)
	return model.SuccessResponse(c, fiber.Map{"buttons": items})
}

// POST /api/admin/quick-buttons
func (h *ContentHandler) AdminCreateQuickButton(c *fiber.Ctx) error {
	var item model.QuickButton
	if err := c.BodyParser(&item); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if item.Title == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "title is required")
	}
	if item.AppID == "" {
		item.AppID = "default"
	}
	if item.Type == "" {
		item.Type = "photo"
	}

	db := database.GetDB()
	if err := db.Create(&item).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create: "+err.Error())
	}
	return model.SuccessResponse(c, item)
}

// PUT /api/admin/quick-buttons/:id
func (h *ContentHandler) AdminUpdateQuickButton(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	var existing model.QuickButton
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}
	var updates model.QuickButton
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	db.Model(&existing).Updates(map[string]interface{}{
		"app_id": nonEmpty(updates.AppID, existing.AppID),
		"type": nonEmpty(updates.Type, existing.Type),
		"title": nonEmpty(updates.Title, existing.Title),
		"icon_url": updates.IconURL,
		"template_id": updates.TemplateID,
		"sort_order": updates.SortOrder,
		"is_active": updates.IsActive,
	})
	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

// DELETE /api/admin/quick-buttons/:id
func (h *ContentHandler) AdminDeleteQuickButton(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	if db.Delete(&model.QuickButton{}, id).RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}
	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// ── Templates ──

// GET /api/admin/templates?app_id=xxx&type=photo|video
func (h *ContentHandler) AdminListTemplates(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "")
	tmplType := strings.TrimSpace(c.Query("type", ""))

	query := db.Model(&model.Template{}).Select("templates.*")
	if appID != "" {
		query = query.Where("templates.app_id = ?", appID)
	}
	if tmplType != "" {
		query = query.Joins("LEFT JOIN categories ON categories.id = templates.category_id")
		if tmplType == "photo" {
			query = query.Where("(templates.category_id = 0 OR categories.type = ? OR categories.type IS NULL OR categories.type = '')", "photo")
		} else {
			query = query.Where("(templates.category_id = 0 OR categories.type = ?)", tmplType)
		}
	}

	var templates []model.Template
	query.Order("templates.sort_order asc, templates.created_at desc").Find(&templates)

	return model.SuccessResponse(c, fiber.Map{
		"templates": templates,
	})
}

// POST /api/admin/templates
func (h *ContentHandler) AdminCreateTemplate(c *fiber.Ctx) error {
	var tmpl model.Template
	if err := c.BodyParser(&tmpl); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body: "+err.Error())
	}

	if tmpl.Name == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "name is required")
	}
	if tmpl.Slug == "" {
		tmpl.Slug = slugify(tmpl.Name)
	}
	if tmpl.AppID == "" {
		tmpl.AppID = "default"
	}

	db := database.GetDB()
	if err := db.Create(&tmpl).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return model.ErrorResponse(c, fiber.StatusConflict, "slug already exists")
		}
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create template: "+err.Error())
	}

	return model.SuccessResponse(c, tmpl)
}

// PUT /api/admin/templates/:id
func (h *ContentHandler) AdminUpdateTemplate(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid template ID")
	}

	db := database.GetDB()
	var existing model.Template
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "template not found")
	}

	var updates model.Template
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	db.Model(&existing).Updates(map[string]interface{}{
		"app_id":           nonEmpty(updates.AppID, existing.AppID),
		"slug":             nonEmpty(updates.Slug, existing.Slug),
		"name":             nonEmpty(updates.Name, existing.Name),
		"description":      updates.Description,
		"action_type":      nonEmpty(updates.ActionType, existing.ActionType),
		"prompt":           nonEmpty(updates.Prompt, existing.Prompt),
		"negative_prompt":  updates.NegativePrompt,
		"provider":         updates.Provider,
		"model":            updates.Model,
		"category_id":      updates.CategoryID,
		"before_media_url":   updates.BeforeMediaURL,
		"before_media_type":  nonEmpty(updates.BeforeMediaType, existing.BeforeMediaType),
		"after_media_url":    updates.AfterMediaURL,
		"after_media_type":   nonEmpty(updates.AfterMediaType, existing.AfterMediaType),
		"reference_video_url": updates.ReferenceVideoURL,
		"icon_url":                updates.IconURL,
		"aspect_ratio":            nonEmpty(updates.AspectRatio, existing.AspectRatio),
		"supported_aspect_ratios": nonEmpty(updates.SupportedAspectRatios, existing.SupportedAspectRatios),
		"params":                  updates.Params,
		"credit_cost":             updates.CreditCost,
		"require_mom_photo":       updates.RequireMomPhoto,
		"require_baby_photo":      updates.RequireBabyPhoto,
		"require_dad_photo":       updates.RequireDadPhoto,
		"is_active":        updates.IsActive,
		"is_featured":      updates.IsFeatured,
		"is_popular":       updates.IsPopular,
		"is_viral":         updates.IsViral,
		"is_premium":       updates.IsPremium,
		"sort_order":       updates.SortOrder,
	})

	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

// DELETE /api/admin/templates/:id
func (h *ContentHandler) AdminDeleteTemplate(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid template ID")
	}

	db := database.GetDB()
	result := db.Delete(&model.Template{}, id)
	if result.RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "template not found")
	}

	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// POST /api/admin/templates/reorder
func (h *ContentHandler) AdminReorderTemplates(c *fiber.Ctx) error {
	type reorderItem struct {
		ID        uint `json:"id"`
		SortOrder int  `json:"sort_order"`
	}
	var body struct {
		Items []reorderItem `json:"items"`
	}
	if err := c.BodyParser(&body); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	db := database.GetDB()
	for _, item := range body.Items {
		if item.ID == 0 {
			continue
		}
		db.Model(&model.Template{}).Where("id = ?", item.ID).Update("sort_order", item.SortOrder)
	}
	return model.SuccessResponse(c, fiber.Map{"reordered": true})
}

// GET /api/admin/onboarding?app_id=xxx
func (h *ContentHandler) AdminListOnboarding(c *fiber.Ctx) error {
	db := database.GetDB()
	appID := c.Query("app_id", "")

	query := db.Model(&model.OnboardingMedia{})
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}

	var media []model.OnboardingMedia
	query.Order("sort_order asc").Find(&media)

	return model.SuccessResponse(c, fiber.Map{
		"onboarding": media,
	})
}

// POST /api/admin/onboarding
func (h *ContentHandler) AdminCreateOnboarding(c *fiber.Ctx) error {
	var media model.OnboardingMedia
	if err := c.BodyParser(&media); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body: "+err.Error())
	}

	if media.MediaURL == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "media_url is required")
	}
	if media.Type == "" {
		media.Type = "video"
	}
	if media.AppID == "" {
		media.AppID = "default"
	}

	db := database.GetDB()
	if err := db.Create(&media).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create: "+err.Error())
	}

	return model.SuccessResponse(c, media)
}

// PUT /api/admin/onboarding/:id
func (h *ContentHandler) AdminUpdateOnboarding(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}

	db := database.GetDB()
	var existing model.OnboardingMedia
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "onboarding media not found")
	}

	var updates model.OnboardingMedia
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	db.Model(&existing).Updates(map[string]interface{}{
		"app_id":        nonEmpty(updates.AppID, existing.AppID),
		"type":          nonEmpty(updates.Type, existing.Type),
		"title":         updates.Title,
		"description":   updates.Description,
		"media_url":     nonEmpty(updates.MediaURL, existing.MediaURL),
		"thumbnail_url": updates.ThumbnailURL,
		"sort_order":    updates.SortOrder,
		"is_active":     updates.IsActive,
	})

	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

// DELETE /api/admin/onboarding/:id
func (h *ContentHandler) AdminDeleteOnboarding(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}

	db := database.GetDB()
	result := db.Delete(&model.OnboardingMedia{}, id)
	if result.RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}

	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// ── Onboarding Reviews ──

// GET /api/admin/reviews
func (h *ContentHandler) AdminListReviews(c *fiber.Ctx) error {
	db := database.GetDB()
	var reviews []model.OnboardingReview
	db.Order("sort_order asc").Find(&reviews)
	return model.SuccessResponse(c, fiber.Map{"reviews": reviews})
}

// POST /api/admin/reviews
func (h *ContentHandler) AdminCreateReview(c *fiber.Ctx) error {
	var review model.OnboardingReview
	if err := c.BodyParser(&review); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if review.Nickname == "" || review.Review == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "nickname and review are required")
	}
	if review.Rating < 1 || review.Rating > 5 {
		review.Rating = 5
	}
	db := database.GetDB()
	if err := db.Create(&review).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create: "+err.Error())
	}
	return model.SuccessResponse(c, review)
}

// PUT /api/admin/reviews/:id
func (h *ContentHandler) AdminUpdateReview(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	var existing model.OnboardingReview
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "review not found")
	}
	var updates model.OnboardingReview
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	db.Model(&existing).Updates(map[string]interface{}{
		"nickname":   nonEmpty(updates.Nickname, existing.Nickname),
		"photo_url":  updates.PhotoURL,
		"review":     nonEmpty(updates.Review, existing.Review),
		"rating":     updates.Rating,
		"sort_order": updates.SortOrder,
		"is_active":  updates.IsActive,
	})
	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

// DELETE /api/admin/reviews/:id
func (h *ContentHandler) AdminDeleteReview(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid ID")
	}
	db := database.GetDB()
	if db.Delete(&model.OnboardingReview{}, id).RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "not found")
	}
	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

// POST /api/admin/upload-media
func (h *ContentHandler) AdminUploadMedia(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "file is required")
	}

	if file.Size > 100*1024*1024 {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "file must be under 100MB")
	}

	if !h.storage.IsReady() {
		return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "R2 storage not configured")
	}

	src, err := file.Open()
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to open file")
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to read file")
	}

	contentType := http.DetectContentType(data)
	if strings.HasSuffix(strings.ToLower(file.Filename), ".svg") || isSVGData(data) {
		contentType = "image/svg+xml"
	}
	ext := guessMediaExtension(contentType, file.Filename)

	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	folder := c.FormValue("folder", "media")
	folder = sanitizeFolder(folder)
	if !isSafeFolder(folder) {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid folder name")
	}
	key := fmt.Sprintf("%s/%s_%s%s", folder, time.Now().Format("20060102_150405"), hex.EncodeToString(randomBytes), ext)

	url, err := h.storage.Upload(c.Context(), key, data, contentType)
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "upload failed: "+err.Error())
	}

	return model.SuccessResponse(c, fiber.Map{
		"url":      url,
		"key":      key,
		"size":     file.Size,
		"mimetype": contentType,
	})
}

// ── Translation ──

// POST /api/admin/translate
func (h *ContentHandler) AdminTranslate(c *fiber.Ctx) error {
	if !h.translate.IsReady() {
		return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "translation service not configured (DEEPSEEK_KEY required)")
	}

	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   uint   `json:"entity_id"`
		SourceLang string `json:"source_lang"`
	}
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.EntityType == "" || req.EntityID == 0 {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "entity_type and entity_id are required")
	}
	if req.SourceLang == "" {
		req.SourceLang = "en"
	}

	db := database.GetDB()

	fields := map[string]string{}
	switch req.EntityType {
	case "template":
		var t model.Template
		if err := db.First(&t, req.EntityID).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusNotFound, "template not found")
		}
		fields["name"] = t.Name
		fields["description"] = t.Description
	case "category":
		var cat model.Category
		if err := db.First(&cat, req.EntityID).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusNotFound, "category not found")
		}
		fields["name"] = cat.Name
		fields["description"] = cat.Description
	case "slider":
		var s model.SliderItem
		if err := db.First(&s, req.EntityID).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusNotFound, "slider not found")
		}
		fields["title"] = s.Title
		fields["description"] = s.Description
	case "onboarding":
		var o model.OnboardingMedia
		if err := db.First(&o, req.EntityID).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusNotFound, "onboarding not found")
		}
		fields["title"] = o.Title
		fields["description"] = o.Description
	case "review":
		var r model.OnboardingReview
		if err := db.First(&r, req.EntityID).Error; err != nil {
			return model.ErrorResponse(c, fiber.StatusNotFound, "review not found")
		}
		fields["review"] = r.Review
	default:
		return model.ErrorResponse(c, fiber.StatusBadRequest, "entity_type must be: template, category, slider, onboarding, review")
	}

	targetLangs := make([]string, 0)
	for _, lang := range service.SupportedLanguages {
		if lang != req.SourceLang {
			targetLangs = append(targetLangs, lang)
		}
	}

	totalSaved := 0
	allTranslations := make(map[string]map[string]string)

	for field, text := range fields {
		if text == "" {
			continue
		}

		translations, err := h.translate.TranslateToAll(c.Context(), text, req.SourceLang, targetLangs)
		if err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "translation failed for "+field+": "+err.Error())
		}

		translations[req.SourceLang] = text

		for lang, value := range translations {
			var existing model.Translation
			result := db.Where("entity_type = ? AND entity_id = ? AND field = ? AND language = ?",
				req.EntityType, req.EntityID, field, lang).First(&existing)

			if result.Error != nil {
				db.Create(&model.Translation{
					EntityType: req.EntityType, EntityID: req.EntityID,
					Field: field, Language: lang, Value: value,
				})
			} else {
				db.Model(&existing).Update("value", value)
			}
			totalSaved++
		}

		allTranslations[field] = translations
	}

	return model.SuccessResponse(c, fiber.Map{
		"entity_type":  req.EntityType,
		"entity_id":    req.EntityID,
		"translations": allTranslations,
		"total_saved":  totalSaved,
		"languages":    len(service.SupportedLanguages),
	})
}

// GET /api/admin/translations?entity_type=template&entity_id=1
func (h *ContentHandler) AdminGetTranslations(c *fiber.Ctx) error {
	entityType := c.Query("entity_type", "")
	entityID := c.QueryInt("entity_id", 0)

	if entityType == "" || entityID == 0 {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "entity_type and entity_id are required")
	}

	db := database.GetDB()
	var translations []model.Translation
	db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Find(&translations)

	grouped := make(map[string]map[string]string)
	for _, t := range translations {
		if grouped[t.Field] == nil {
			grouped[t.Field] = make(map[string]string)
		}
		grouped[t.Field][t.Language] = t.Value
	}

	return model.SuccessResponse(c, fiber.Map{
		"entity_type":  entityType,
		"entity_id":    entityID,
		"translations": grouped,
	})
}

// PUT /api/admin/translations — manually set a translation for a specific entity/field/language
func (h *ContentHandler) AdminSetTranslation(c *fiber.Ctx) error {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   uint   `json:"entity_id"`
		Field      string `json:"field"`
		Language   string `json:"language"`
		Value      string `json:"value"`
	}
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.EntityType == "" || req.EntityID == 0 || req.Field == "" || req.Language == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "entity_type, entity_id, field, and language are required")
	}

	db := database.GetDB()
	var existing model.Translation
	result := db.Where("entity_type = ? AND entity_id = ? AND field = ? AND language = ?",
		req.EntityType, req.EntityID, req.Field, req.Language).First(&existing)

	if result.Error != nil {
		t := model.Translation{
			EntityType: req.EntityType, EntityID: req.EntityID,
			Field: req.Field, Language: req.Language, Value: req.Value,
		}
		db.Create(&t)
		return model.SuccessResponse(c, t)
	}

	db.Model(&existing).Update("value", req.Value)
	existing.Value = req.Value
	return model.SuccessResponse(c, existing)
}

// POST /api/admin/translate-all — translate ALL missing translations for all entities in one click
func (h *ContentHandler) AdminTranslateAll(c *fiber.Ctx) error {
	if !h.translate.IsReady() {
		return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "translation service not configured (DEEPSEEK_KEY required)")
	}

	var req struct {
		SourceLang string `json:"source_lang"`
		EntityType string `json:"entity_type"` // optional: filter to specific type
	}
	if err := c.BodyParser(&req); err != nil {
		req.SourceLang = "en"
	}
	if req.SourceLang == "" {
		req.SourceLang = "en"
	}

	db := database.GetDB()

	type entityEntry struct {
		entityType string
		entityID   uint
		fields     map[string]string // field -> source text
	}

	var entries []entityEntry

	// Collect all entities that need translation
	shouldProcess := func(t string) bool {
		return req.EntityType == "" || req.EntityType == t
	}

	if shouldProcess("template") {
		var templates []model.Template
		db.Where("is_active = ?", true).Find(&templates)
		for _, t := range templates {
			fields := map[string]string{}
			if t.Name != "" {
				fields["name"] = t.Name
			}
			if t.Description != "" {
				fields["description"] = t.Description
			}
			if len(fields) > 0 {
				entries = append(entries, entityEntry{"template", t.ID, fields})
			}
		}
	}

	if shouldProcess("category") {
		var categories []model.Category
		db.Where("is_active = ?", true).Find(&categories)
		for _, cat := range categories {
			fields := map[string]string{}
			if cat.Name != "" {
				fields["name"] = cat.Name
			}
			if cat.Description != "" {
				fields["description"] = cat.Description
			}
			if len(fields) > 0 {
				entries = append(entries, entityEntry{"category", cat.ID, fields})
			}
		}
	}

	if shouldProcess("slider") {
		var sliders []model.SliderItem
		db.Where("is_active = ?", true).Find(&sliders)
		for _, s := range sliders {
			fields := map[string]string{}
			if s.Title != "" {
				fields["title"] = s.Title
			}
			if s.Description != "" {
				fields["description"] = s.Description
			}
			if len(fields) > 0 {
				entries = append(entries, entityEntry{"slider", s.ID, fields})
			}
		}
	}

	if shouldProcess("onboarding") {
		var media []model.OnboardingMedia
		db.Where("is_active = ?", true).Find(&media)
		for _, o := range media {
			fields := map[string]string{}
			if o.Title != "" {
				fields["title"] = o.Title
			}
			if o.Description != "" {
				fields["description"] = o.Description
			}
			if len(fields) > 0 {
				entries = append(entries, entityEntry{"onboarding", o.ID, fields})
			}
		}
	}

	if shouldProcess("review") {
		var reviews []model.OnboardingReview
		db.Where("is_active = ?", true).Find(&reviews)
		for _, r := range reviews {
			fields := map[string]string{}
			if r.Review != "" {
				fields["review"] = r.Review
			}
			if len(fields) > 0 {
				entries = append(entries, entityEntry{"review", r.ID, fields})
			}
		}
	}

	// Load existing translations to find what's missing
	var allTranslations []model.Translation
	db.Find(&allTranslations)

	existingSet := make(map[string]bool)
	for _, t := range allTranslations {
		key := fmt.Sprintf("%s:%d:%s:%s", t.EntityType, t.EntityID, t.Field, t.Language)
		existingSet[key] = true
	}

	// Determine target languages
	targetLangs := make([]string, 0)
	for _, lang := range service.SupportedLanguages {
		if lang != req.SourceLang {
			targetLangs = append(targetLangs, lang)
		}
	}

	totalTranslated := 0
	totalSkipped := 0
	var errors []string

	for _, entry := range entries {
		for field, text := range entry.fields {
			// Find which languages are missing for this entity/field
			missingLangs := make([]string, 0)
			for _, lang := range targetLangs {
				key := fmt.Sprintf("%s:%d:%s:%s", entry.entityType, entry.entityID, field, lang)
				if !existingSet[key] {
					missingLangs = append(missingLangs, lang)
				}
			}

			if len(missingLangs) == 0 {
				totalSkipped++
				continue
			}

			// Translate to missing languages
			translations, err := h.translate.TranslateToAll(c.Context(), text, req.SourceLang, missingLangs)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s:%d:%s - %s", entry.entityType, entry.entityID, field, err.Error()))
				continue
			}

			// Save translations
			for lang, value := range translations {
				if value == "" {
					continue
				}
				var existing model.Translation
				result := db.Where("entity_type = ? AND entity_id = ? AND field = ? AND language = ?",
					entry.entityType, entry.entityID, field, lang).First(&existing)

				if result.Error != nil {
					db.Create(&model.Translation{
						EntityType: entry.entityType, EntityID: entry.entityID,
						Field: field, Language: lang, Value: value,
					})
				} else {
					db.Model(&existing).Update("value", value)
				}
				totalTranslated++
			}
		}
	}

	response := fiber.Map{
		"total_entities":   len(entries),
		"total_translated": totalTranslated,
		"total_skipped":    totalSkipped,
		"target_languages": len(targetLangs),
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	return model.SuccessResponse(c, response)
}

// ─── Translation helpers for mobile API ─────────────────────

func (h *ContentHandler) applyTranslations(items interface{}, entityType, lang string) {
	db := database.GetDB()

	switch v := items.(type) {
	case []model.Template:
		ids := make([]uint, len(v))
		for i := range v {
			ids[i] = v[i].ID
		}
		transMap := h.loadTranslationMap(db, entityType, ids, lang)
		for i := range v {
			if m, ok := transMap[v[i].ID]; ok {
				if val, ok := m["name"]; ok {
					v[i].Name = val
				}
				if val, ok := m["description"]; ok {
					v[i].Description = val
				}
			}
		}
	case []model.Category:
		ids := make([]uint, len(v))
		for i := range v {
			ids[i] = v[i].ID
		}
		transMap := h.loadTranslationMap(db, entityType, ids, lang)
		for i := range v {
			if m, ok := transMap[v[i].ID]; ok {
				if val, ok := m["name"]; ok {
					v[i].Name = val
				}
				if val, ok := m["description"]; ok {
					v[i].Description = val
				}
			}
		}
	case []model.SliderItem:
		ids := make([]uint, len(v))
		for i := range v {
			ids[i] = v[i].ID
		}
		transMap := h.loadTranslationMap(db, entityType, ids, lang)
		for i := range v {
			if m, ok := transMap[v[i].ID]; ok {
				if val, ok := m["title"]; ok {
					v[i].Title = val
				}
				if val, ok := m["description"]; ok {
					v[i].Description = val
				}
			}
		}
	case []model.OnboardingMedia:
		ids := make([]uint, len(v))
		for i := range v {
			ids[i] = v[i].ID
		}
		transMap := h.loadTranslationMap(db, entityType, ids, lang)
		for i := range v {
			if m, ok := transMap[v[i].ID]; ok {
				if val, ok := m["title"]; ok {
					v[i].Title = val
				}
				if val, ok := m["description"]; ok {
					v[i].Description = val
				}
			}
		}
	case []model.OnboardingReview:
		ids := make([]uint, len(v))
		for i := range v {
			ids[i] = v[i].ID
		}
		transMap := h.loadTranslationMap(db, entityType, ids, lang)
		for i := range v {
			if m, ok := transMap[v[i].ID]; ok {
				if val, ok := m["review"]; ok {
					v[i].Review = val
				}
			}
		}
	}
}

func (h *ContentHandler) loadTranslationMap(db *gorm.DB, entityType string, ids []uint, lang string) map[uint]map[string]string {
	result := make(map[uint]map[string]string)
	if len(ids) == 0 {
		return result
	}
	var translations []model.Translation
	db.Where("entity_type = ? AND entity_id IN ? AND language = ?", entityType, ids, lang).Find(&translations)
	for _, t := range translations {
		if result[t.EntityID] == nil {
			result[t.EntityID] = make(map[string]string)
		}
		result[t.EntityID][t.Field] = t.Value
	}
	return result
}

// ─── Helpers ────────────────────────────────────────────────

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	var clean []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean = append(clean, r)
		}
	}
	return string(clean)
}

func nonEmpty(newVal, existing string) string {
	if newVal != "" {
		return newVal
	}
	return existing
}

func (h *ContentHandler) AdminListNotes(c *fiber.Ctx) error {
	db := database.GetDB()
	var notes []model.Note
	db.Order("sort_order asc, created_at desc").Find(&notes)
	return model.SuccessResponse(c, fiber.Map{"notes": notes})
}

func (h *ContentHandler) AdminCreateNote(c *fiber.Ctx) error {
	var note model.Note
	if err := c.BodyParser(&note); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if note.Title == "" {
		note.Title = "Untitled"
	}
	db := database.GetDB()
	if err := db.Create(&note).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to create note: "+err.Error())
	}
	return model.SuccessResponse(c, note)
}

func (h *ContentHandler) AdminUpdateNote(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid id")
	}
	db := database.GetDB()
	var existing model.Note
	if err := db.First(&existing, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "note not found")
	}
	var updates model.Note
	if err := c.BodyParser(&updates); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	db.Model(&existing).Updates(map[string]interface{}{
		"title":      updates.Title,
		"content":    updates.Content,
		"color":      updates.Color,
		"sort_order": updates.SortOrder,
	})
	db.First(&existing, id)
	return model.SuccessResponse(c, existing)
}

func (h *ContentHandler) AdminDeleteNote(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid id")
	}
	db := database.GetDB()
	if db.Delete(&model.Note{}, id).RowsAffected == 0 {
		return model.ErrorResponse(c, fiber.StatusNotFound, "note not found")
	}
	return model.SuccessResponse(c, fiber.Map{"deleted": true})
}

func guessMediaExtension(contentType, filename string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return ".jpg"
	case strings.Contains(contentType, "mp4"), strings.Contains(contentType, "video"):
		return ".mp4"
	case strings.Contains(contentType, "svg"):
		return ".svg"
	case strings.Contains(contentType, "webm"):
		return ".webm"
	case strings.Contains(contentType, "mov"), strings.Contains(contentType, "quicktime"):
		return ".mov"
	}

	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		return filename[idx:]
	}
	return ".bin"
}

func isSVGData(data []byte) bool {
	n := len(data)
	if n > 512 {
		n = 512
	}
	prefix := strings.ToLower(string(data[:n]))
	return strings.Contains(prefix, "<svg")
}
