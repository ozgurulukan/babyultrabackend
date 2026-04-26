package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/ozgurulukan/bubsiebackend/internal/database"
	"github.com/ozgurulukan/bubsiebackend/internal/model"
	"github.com/ozgurulukan/bubsiebackend/internal/service"
	"gorm.io/gorm"
)

var errInstallSeedRequired = errors.New("install seed required")

func FirebaseAuth(firebase *service.FirebaseService, initialCredits int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !firebase.IsReady() {
			return model.ErrorResponse(c, fiber.StatusServiceUnavailable, "authentication service not configured")
		}

		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "missing authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "invalid authorization format, expected: Bearer <token>")
		}

		token, err := firebase.VerifyIDToken(c.Context(), parts[1])
		if err != nil {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "invalid or expired token")
		}

		c.Locals("uid", token.UID)
		c.Locals("email", token.Claims["email"])
		c.Locals("token", token)

		deviceID := strings.TrimSpace(c.Get("X-Device-ID"))
		if deviceID != "" && isDeviceBanned(deviceID) {
			return model.ErrorResponse(c, fiber.StatusForbidden, "this device has been restricted from using our services")
		}

		if isUserBanned(token.UID) {
			return model.ErrorResponse(c, fiber.StatusForbidden, "your account has been suspended")
		}

		installSeed := strings.TrimSpace(c.Get("X-Install-Seed"))
		if err := upsertUser(token.UID, token.Claims, installSeed, deviceID, initialCredits); err != nil {
			if errors.Is(err, errInstallSeedRequired) {
				return model.ErrorResponse(c, fiber.StatusBadRequest, "missing X-Install-Seed header")
			}
			log.Printf("User upsert failed [uid=%s]: %v", token.UID, err)
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to upsert user")
		}

		return c.Next()
	}
}

var (
	googleCerts     map[string]string
	googleCertsMu   sync.RWMutex
	googleCertsTime time.Time
)

func fetchGoogleCerts() (map[string]string, error) {
	googleCertsMu.RLock()
	if googleCerts != nil && time.Since(googleCertsTime) < 1*time.Hour {
		certs := googleCerts
		googleCertsMu.RUnlock()
		return certs, nil
	}
	googleCertsMu.RUnlock()

	googleCertsMu.Lock()
	defer googleCertsMu.Unlock()

	if googleCerts != nil && time.Since(googleCertsTime) < 1*time.Hour {
		return googleCerts, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch google certs: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read google certs: %w", err)
	}

	var certs map[string]string
	if err := json.Unmarshal(body, &certs); err != nil {
		return nil, fmt.Errorf("parse google certs: %w", err)
	}

	googleCerts = certs
	googleCertsTime = time.Now()
	return certs, nil
}

func LightweightFirebaseAuth(projectID string, adminEmail string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "missing authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "invalid authorization format")
		}

		tokenString := parts[1]

		certs, err := fetchGoogleCerts()
		if err != nil {
			return model.ErrorResponse(c, fiber.StatusInternalServerError, "failed to fetch auth certificates")
		}

		var parsedToken *jwt.Token
		for _, certPEM := range certs {
			key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(certPEM))
			if err != nil {
				continue
			}
			parsedToken, err = jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return key, nil
			},
				jwt.WithIssuer("https://securetoken.google.com/"+projectID),
				jwt.WithAudience(projectID),
			)
			if err == nil {
				break
			}
		}

		if parsedToken == nil || !parsedToken.Valid {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "invalid or expired token")
		}

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		if !ok {
			return model.ErrorResponse(c, fiber.StatusUnauthorized, "invalid token claims")
		}

		uid, _ := claims["user_id"].(string)
		email, _ := claims["email"].(string)

		c.Locals("uid", uid)
		c.Locals("email", email)

		if adminEmail != "" && email != adminEmail {
			return model.ErrorResponse(c, fiber.StatusForbidden, "you do not have access to this panel")
		}

		if isUserBanned(uid) {
			return model.ErrorResponse(c, fiber.StatusForbidden, "your account has been suspended")
		}

		go func() {
			_ = upsertUser(uid, map[string]interface{}{
				"email":   email,
				"name":    claims["name"],
				"picture": claims["picture"],
			}, "", "", 0)
		}()

		return c.Next()
	}
}

func isUserBanned(uid string) bool {
	db := database.GetDB()
	if db == nil || uid == "" {
		return false
	}
	var user model.User
	if err := db.Select("is_banned", "deleted_at").Where("firebase_uid = ?", uid).First(&user).Error; err != nil {
		return false
	}
	if user.DeletedAt != nil {
		return true
	}
	return user.IsBanned
}

func isDeviceBanned(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	db := database.GetDB()
	if db == nil {
		return false
	}
	var ban model.DeviceBan
	if err := db.Where("device_id = ?", deviceID).First(&ban).Error; err != nil {
		return false
	}
	return true
}

func upsertUser(uid string, claims map[string]interface{}, installSeed string, deviceID string, initialCredits int) error {
	db := database.GetDB()
	if db == nil || uid == "" {
		return nil
	}

	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)
	picture, _ := claims["picture"].(string)

	updates := map[string]interface{}{
		"email":                  email,
		"name":                   name,
		"photo_url":              picture,
		"revenuecat_customer_id": uid,
		"last_login":             time.Now(),
	}
	if deviceID != "" {
		updates["device_id"] = deviceID
	}

	var user model.User
	result := db.Where("firebase_uid = ?", uid).First(&user)
	if result.Error == nil {
		return db.Model(&user).Updates(updates).Error
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}

	credits := 0
	installSeed = strings.TrimSpace(installSeed)
	if installSeed == "" {
		if initialCredits > 0 {
			return errInstallSeedRequired
		}
	} else {
		var err error
		credits, err = claimInitialCredits(db, uid, installSeed, initialCredits)
		if err != nil {
			return err
		}
	}

	createErr := db.Create(&model.User{
		FirebaseUID:          uid,
		Email:                email,
		Name:                 name,
		PhotoURL:             picture,
		DeviceID:             deviceID,
		RevenueCatCustomerID: uid,
		Credits:              credits,
		LastLogin:            time.Now(),
	}).Error
	if createErr == nil {
		return nil
	}

	var existing model.User
	if err := db.Where("firebase_uid = ?", uid).First(&existing).Error; err != nil {
		return createErr
	}

	return db.Model(&existing).Updates(updates).Error
}

func claimInitialCredits(db *gorm.DB, uid, installSeed string, initialCredits int) (int, error) {
	if initialCredits < 0 {
		initialCredits = 0
	}

	sum := sha256.Sum256([]byte(installSeed))
	seedHash := hex.EncodeToString(sum[:])

	var claim model.InstallCreditClaim
	res := db.Where("install_seed_hash = ?", seedHash).First(&claim)
	if res.Error == nil {
		return 0, nil
	}
	if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return 0, res.Error
	}

	createErr := db.Create(&model.InstallCreditClaim{
		InstallSeedHash:  seedHash,
		FirstFirebaseUID: uid,
	}).Error
	if createErr != nil {
		if err := db.Where("install_seed_hash = ?", seedHash).First(&claim).Error; err == nil {
			return 0, nil
		}
		return 0, createErr
	}

	return initialCredits, nil
}
