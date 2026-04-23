package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ozgurulukan/bubsiebackend/internal/config"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service"
	"github.com/ozgurulukan/bubsiebackend/internal/service/provider"
)

var allProviderNames = []string{"fal.ai", "replicate", "deepseek", "openrouter", "gemini"}

type AdminHandler struct {
	cfg        *config.Config
	registry   *provider.Registry
	firebase   *service.FirebaseService
	revenuecat *service.RevenueCatService
}

func NewAdminHandler(
	cfg *config.Config,
	registry *provider.Registry,
	firebase *service.FirebaseService,
	revenuecat *service.RevenueCatService,
) *AdminHandler {
	return &AdminHandler{
		cfg:        cfg,
		registry:   registry,
		firebase:   firebase,
		revenuecat: revenuecat,
	}
}

func (h *AdminHandler) GetStats(c *fiber.Ctx) error {
	db := database.GetDB()

	// User count: try Firebase first, fallback to DB
	var userTotal int64
	firebaseStatus := "not_configured"
	if h.firebase.IsReady() {
		count, err := h.firebase.GetUserCount(c.Context())
		if err == nil {
			userTotal = int64(count.Total)
			firebaseStatus = "connected"
		} else {
			firebaseStatus = "error"
		}
	}
	if userTotal == 0 {
		db.Model(&model.User{}).Count(&userTotal)
	}

	revenue, err := h.revenuecat.GetOverview(c.Context())
	if err != nil {
		revenue = &service.RevenueStats{
			LastUpdated: time.Now().UTC().Format(time.RFC3339),
		}
	}

	var totalRequests int64
	db.Model(&model.RequestLog{}).Count(&totalRequests)

	var todayRequests int64
	today := time.Now().UTC().Truncate(24 * time.Hour)
	db.Model(&model.RequestLog{}).Where("created_at >= ?", today).Count(&todayRequests)

	var providerStats []struct {
		Provider string `json:"provider"`
		Count    int64  `json:"count"`
	}
	db.Model(&model.RequestLog{}).
		Select("provider, count(*) as count").
		Group("provider").
		Scan(&providerStats)

	return model.SuccessResponse(c, fiber.Map{
		"users":           fiber.Map{"total": userTotal},
		"firebase_status": firebaseStatus,
		"revenue":         revenue,
		"total_requests":  totalRequests,
		"today_requests":  todayRequests,
		"provider_usage":  providerStats,
	})
}

func (h *AdminHandler) GetRevenue(c *fiber.Ctx) error {
	revenue, err := h.revenuecat.GetOverview(c.Context())
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to get revenue: "+err.Error())
	}
	return model.SuccessResponse(c, revenue)
}

func (h *AdminHandler) GetRevenueDetailed(c *fiber.Ctx) error {
	db := database.GetDB()
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	weekStart := today.AddDate(0, 0, -int(today.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonthStart := monthStart.AddDate(0, -1, 0)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

	countSince := func(since time.Time) int64 {
		var cnt int64
		db.Model(&model.RequestLog{}).Where("created_at >= ?", since).Count(&cnt)
		return cnt
	}

	var userTotal int64
	db.Model(&model.User{}).Count(&userTotal)
	var proUsers int64
	db.Model(&model.User{}).Where("is_pro = ?", true).Count(&proUsers)

	var last30Days []struct {
		Day   string `json:"day"`
		Count int64  `json:"count"`
	}
	db.Model(&model.RequestLog{}).
		Select("DATE(created_at) as day, COUNT(*) as count").
		Where("created_at >= ?", today.AddDate(0, 0, -30)).
		Group("DATE(created_at)").
		Order("day asc").
		Scan(&last30Days)

	revenue, _ := h.revenuecat.GetOverview(c.Context())
	if revenue == nil {
		revenue = &service.RevenueStats{LastUpdated: now.Format(time.RFC3339)}
	}

	var lastMonthRequests int64
	db.Model(&model.RequestLog{}).Where("created_at >= ? AND created_at < ?", lastMonthStart, monthStart).Count(&lastMonthRequests)

	return model.SuccessResponse(c, fiber.Map{
		"revenuecat": revenue,
		"usage": fiber.Map{
			"today":      countSince(today),
			"this_week":  countSince(weekStart),
			"this_month": countSince(monthStart),
			"last_month": lastMonthRequests,
			"this_year":  countSince(yearStart),
		},
		"users": fiber.Map{
			"total": userTotal,
			"pro":   proUsers,
			"free":  userTotal - proUsers,
		},
		"chart_data": last30Days,
	})
}

func (h *AdminHandler) GetUserCount(c *fiber.Ctx) error {
	db := database.GetDB()
	var total int64

	if h.firebase.IsReady() {
		count, err := h.firebase.GetUserCount(c.Context())
		if err == nil {
			return model.SuccessResponse(c, count)
		}
	}

	db.Model(&model.User{}).Count(&total)
	return model.SuccessResponse(c, fiber.Map{"total": total, "source": "database"})
}

func (h *AdminHandler) HealthCheckProviders(c *fiber.Ctx) error {
	results := h.registry.HealthCheckAll(c.Context())

	// Include unconfigured providers too
	configured := make(map[string]bool)
	for _, r := range results {
		configured[r.Provider] = true
	}
	for _, name := range allProviderNames {
		if !configured[name] {
			results = append(results, provider.HealthStatus{
				Provider: name,
				Active:   false,
				Message:  "not configured (no API key)",
			})
		}
	}

	return model.SuccessResponse(c, fiber.Map{
		"providers": results,
	})
}

func (h *AdminHandler) TestProvider(c *fiber.Ctx) error {
	var req model.ProviderTestRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Provider == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "provider is required")
	}

	p, err := h.registry.Get(req.Provider)
	if err != nil {
		return model.SuccessResponse(c, provider.HealthStatus{
			Provider: req.Provider,
			Active:   false,
			Message:  "not configured (no API key)",
		})
	}

	status := p.HealthCheck(c.Context())
	return model.SuccessResponse(c, status)
}

func (h *AdminHandler) ListProviders(c *fiber.Ctx) error {
	configured := h.registry.List()
	configuredMap := make(map[string]bool)
	for _, name := range configured {
		configuredMap[name] = true
	}

	db := database.GetDB()

	type rawSetting struct {
		Provider string
		IsActive bool
	}
	var rawSettings []rawSetting
	db.Table("provider_settings").Select("provider, is_active").Find(&rawSettings)

	settingsMap := make(map[string]bool)
	for _, rs := range rawSettings {
		settingsMap[rs.Provider] = rs.IsActive
	}

	type providerInfo struct {
		Name       string `json:"name"`
		Configured bool   `json:"configured"`
		Enabled    bool   `json:"enabled"`
	}

	list := make([]providerInfo, 0, len(allProviderNames))
	for _, name := range allProviderNames {
		enabled := configuredMap[name]
		if isActive, ok := settingsMap[name]; ok {
			enabled = isActive
		}
		list = append(list, providerInfo{
			Name:       name,
			Configured: configuredMap[name],
			Enabled:    enabled,
		})
	}

	return model.SuccessResponse(c, fiber.Map{
		"providers": list,
	})
}

func (h *AdminHandler) ToggleProvider(c *fiber.Ctx) error {
	var req model.ToggleProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Provider == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "provider is required")
	}

	db := database.GetDB()
	var count int64
	db.Table("provider_settings").Where("provider = ?", req.Provider).Count(&count)

	if count == 0 {
		db.Exec("INSERT INTO provider_settings (provider, is_active, models, updated_at) VALUES (?, ?, '[]', ?)",
			req.Provider, req.Enabled, time.Now())
	} else {
		db.Exec("UPDATE provider_settings SET is_active = ?, updated_at = ? WHERE provider = ?",
			req.Enabled, time.Now(), req.Provider)
	}

	return model.SuccessResponse(c, fiber.Map{
		"provider": req.Provider,
		"enabled":  req.Enabled,
	})
}

func (h *AdminHandler) UpdateProviderKey(c *fiber.Ctx) error {
	var req model.UpdateKeysRequest
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.Provider == "" || req.Key == "" {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "provider and key are required")
	}

	db := database.GetDB()
	var setting model.ProviderSetting
	result := db.Where("provider = ?", req.Provider).First(&setting)

	if result.Error != nil {
		db.Create(&model.ProviderSetting{
			Provider: req.Provider,
			APIKey:   req.Key,
			IsActive: true,
		})
	} else {
		db.Model(&setting).Updates(map[string]interface{}{
			"api_key":    req.Key,
			"updated_at": time.Now(),
		})
	}

	return model.SuccessResponse(c, fiber.Map{
		"message": "provider key updated successfully",
	})
}

func (h *AdminHandler) GetRequestLogs(c *fiber.Ctx) error {
	db := database.GetDB()

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	var logs []model.RequestLog
	var total int64

	db.Model(&model.RequestLog{}).Count(&total)
	db.Order("created_at desc").Offset(offset).Limit(limit).Find(&logs)

	return model.SuccessResponse(c, fiber.Map{
		"logs":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// ─── User Management ────────────────────────────────────────

func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	db := database.GetDB()

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit
	search := c.Query("search", "")

	var users []model.User
	var total int64

	query := db.Model(&model.User{})
	if search != "" {
		query = query.Where("email LIKE ? OR name LIKE ? OR firebase_uid LIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	query.Count(&total)
	query.Order("created_at desc").Offset(offset).Limit(limit).Find(&users)

	type userRow struct {
		ID          uint   `json:"id"`
		FirebaseUID string `json:"firebase_uid"`
		Email       string `json:"email"`
		Name        string `json:"name"`
		PhotoURL    string `json:"photo_url"`
		Credits     int    `json:"credits"`
		IsPro       bool   `json:"is_pro"`
		IsBanned    bool   `json:"is_banned"`
		BanReason   string `json:"ban_reason"`
		TotalUsage  int64  `json:"total_usage"`
		LastLogin   string `json:"last_login"`
		CreatedAt   string `json:"created_at"`
	}

	rows := make([]userRow, 0, len(users))
	for _, u := range users {
		var usage int64
		db.Model(&model.RequestLog{}).Where("firebase_uid = ?", u.FirebaseUID).Count(&usage)
		rows = append(rows, userRow{
			ID: u.ID, FirebaseUID: u.FirebaseUID,
			Email: u.Email, Name: u.Name, PhotoURL: u.PhotoURL,
			Credits: u.Credits, IsPro: u.IsPro,
			IsBanned: u.IsBanned, BanReason: u.BanReason,
			TotalUsage: usage,
			LastLogin: u.LastLogin.UTC().Format(time.RFC3339),
			CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	return model.SuccessResponse(c, fiber.Map{
		"users": rows,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *AdminHandler) UpdateUserCredits(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid user ID")
	}

	var req struct {
		Credits   int    `json:"credits"`
		IsPro     *bool  `json:"is_pro"`
		IsBanned  *bool  `json:"is_banned"`
		BanReason string `json:"ban_reason"`
	}
	if err := c.BodyParser(&req); err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}

	db := database.GetDB()
	var user model.User
	if err := db.First(&user, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "user not found")
	}

	updates := map[string]interface{}{
		"credits": req.Credits,
	}
	if req.IsPro != nil {
		updates["is_pro"] = *req.IsPro
	}
	if req.IsBanned != nil {
		updates["is_banned"] = *req.IsBanned
		updates["ban_reason"] = req.BanReason
	}

	db.Model(&user).Updates(updates)
	db.First(&user, id)

	return model.SuccessResponse(c, fiber.Map{
		"id":         user.ID,
		"email":      user.Email,
		"credits":    user.Credits,
		"is_pro":     user.IsPro,
		"is_banned":  user.IsBanned,
		"ban_reason": user.BanReason,
	})
}

func (h *AdminHandler) DeleteUser(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return model.ErrorResponse(c, fiber.StatusBadRequest, "invalid user ID")
	}

	db := database.GetDB()
	var user model.User
	if err := db.First(&user, id).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusNotFound, "user not found")
	}

	if err := db.Delete(&user).Error; err != nil {
		return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to delete user")
	}

	return model.SuccessResponse(c, fiber.Map{
		"id":    user.ID,
		"email": user.Email,
	})
}
