package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const revenuecatBaseURL = "https://api.revenuecat.com/v2"

type RevenueCatService struct {
	apiKey          string
	projectID       string
	client          *http.Client
	productIDCache  map[string]string // RevenueCat product ID -> store identifier
}

// CustomerInfo represents RevenueCat subscriber info (V2 format)
type CustomerInfo struct {
	Entitlements struct {
		Pro struct {
			IsActive bool `json:"is_active"`
		} `json:"pro"`
	} `json:"entitlements"`
	NonSubscriptionTransactions []struct {
		ProductID string `json:"product_id"`
	} `json:"non_subscription_transactions"`
}

func NewRevenueCatService(apiKey, projectID string) *RevenueCatService {
	return &RevenueCatService{
		apiKey:         apiKey,
		projectID:      projectID,
		client:         &http.Client{Timeout: 15 * time.Second},
		productIDCache: make(map[string]string),
	}
}

// loadProductMapping fetches RevenueCat products and maps internal IDs to store identifiers.
func (r *RevenueCatService) loadProductMapping(ctx context.Context) error {
	url := fmt.Sprintf("%s/projects/%s/products", revenuecatBaseURL, r.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("products API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []struct {
			ID              string `json:"id"`
			StoreIdentifier string `json:"store_identifier"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	for _, p := range result.Items {
		r.productIDCache[p.ID] = p.StoreIdentifier
	}
	fmt.Printf("[RevenueCat] Loaded %d product mappings\n", len(r.productIDCache))
	return nil
}

type RevenueStats struct {
	Revenue       float64 `json:"revenue"`
	ActiveSubs    int     `json:"active_subscriptions"`
	MRR           float64 `json:"mrr"`
	TrialCount    int     `json:"trial_count"`
	LastUpdated   string  `json:"last_updated"`
}

func (r *RevenueCatService) GetOverview(ctx context.Context) (*RevenueStats, error) {
	if r.apiKey == "" {
		return nil, fmt.Errorf("revenuecat: API key not configured")
	}

	url := fmt.Sprintf("%s/projects/%s/metrics/overview", revenuecatBaseURL, r.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("revenuecat: API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("revenuecat: unmarshal error: %w", err)
	}

	fmt.Printf("[RevenueCat] Overview raw response: %s\n", string(body))

	stats := &RevenueStats{
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
	}

	// Helper to safely read a number from interface{}
	readNumber := func(v interface{}) float64 {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case string:
			f, _ := strconv.ParseFloat(n, 64)
			return f
		case map[string]interface{}:
			// nested object e.g. {"value": 123}
			if val, ok := n["value"].(float64); ok {
				return val
			}
		}
		return 0
	}

	// Try metrics as object first
	if metrics, ok := result["metrics"].(map[string]interface{}); ok {
		stats.MRR = readNumber(metrics["mrr"])
		stats.Revenue = readNumber(metrics["revenue"])
		stats.ActiveSubs = int(readNumber(metrics["active_subscriptions"]))
		stats.TrialCount = int(readNumber(metrics["active_trials"]))
	}

	// Also check root-level fields as fallback
	if stats.MRR == 0 {
		stats.MRR = readNumber(result["mrr"])
	}
	if stats.Revenue == 0 {
		stats.Revenue = readNumber(result["revenue"])
	}
	if stats.ActiveSubs == 0 {
		stats.ActiveSubs = int(readNumber(result["active_subscriptions"]))
	}
	if stats.TrialCount == 0 {
		stats.TrialCount = int(readNumber(result["active_trials"]))
	}

	// Try metrics as array (older v2 format or different endpoint)
	if stats.MRR == 0 && stats.Revenue == 0 && stats.ActiveSubs == 0 {
		if metricsArr, ok := result["metrics"].([]interface{}); ok {
			for _, m := range metricsArr {
				metric, ok := m.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := metric["name"].(string)
				value := readNumber(metric["value"])
				switch name {
				case "revenue":
					stats.Revenue = value
				case "active_subscriptions":
					stats.ActiveSubs = int(value)
				case "mrr":
					stats.MRR = value
				case "active_trials":
					stats.TrialCount = int(value)
				}
			}
		}
	}

	fmt.Printf("[RevenueCat] Parsed stats: MRR=%.2f Revenue=%.2f ActiveSubs=%d Trials=%d\n",
		stats.MRR, stats.Revenue, stats.ActiveSubs, stats.TrialCount)

	return stats, nil
}

// GetCustomerInfo fetches a subscriber's purchase info from RevenueCat REST API v2.
// Requires a Secret API key (starts with sk_).
func (r *RevenueCatService) GetCustomerInfo(ctx context.Context, appUserID string) (*CustomerInfo, error) {
	if r.apiKey == "" {
		return nil, fmt.Errorf("revenuecat: API key not configured")
	}
	if r.projectID == "" {
		return nil, fmt.Errorf("revenuecat: Project ID not configured")
	}

	url := fmt.Sprintf("%s/projects/%s/customers/%s", revenuecatBaseURL, r.projectID, appUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("revenuecat: API error (status %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("[RevenueCat] GetCustomerInfo raw response: %s\n", string(body))

	// RevenueCat v2 GetCustomer returns active_entitlements at root level (no "customer" wrapper)
	var v2Result struct {
		ActiveEntitlements struct {
			Items []struct {
				EntitlementID string `json:"entitlement_id"`
				ExpiresAt     int64  `json:"expires_at"`
			} `json:"items"`
		} `json:"active_entitlements"`
	}

	if err := json.Unmarshal(body, &v2Result); err != nil {
		return nil, fmt.Errorf("revenuecat: unmarshal error: %w", err)
	}

	var info CustomerInfo
	// V2: active_entitlements.items indicates active entitlement status (check expiry)
	now := time.Now().UnixMilli()
	for _, e := range v2Result.ActiveEntitlements.Items {
		if e.ExpiresAt == 0 || e.ExpiresAt > now {
			info.Entitlements.Pro.IsActive = true
			break
		}
	}
	fmt.Printf("[RevenueCat] Parsed: isPro=%v\n", info.Entitlements.Pro.IsActive)
	return &info, nil
}

// PurchaseItem represents a single purchase from RevenueCat V2 API.
type PurchaseItem struct {
	ID          string `json:"id"`
	ProductID   string `json:"product_id"`
	Store       string `json:"store"`
	PurchasedAt int64  `json:"purchased_at"`
}

// GetCustomerPurchases fetches one-time purchases from RevenueCat REST API v2.
func (r *RevenueCatService) GetCustomerPurchases(ctx context.Context, appUserID string) ([]PurchaseItem, error) {
	if r.apiKey == "" {
		return nil, fmt.Errorf("revenuecat: API key not configured")
	}
	if r.projectID == "" {
		return nil, fmt.Errorf("revenuecat: Project ID not configured")
	}

	url := fmt.Sprintf("%s/projects/%s/customers/%s/purchases", revenuecatBaseURL, r.projectID, appUserID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("revenuecat: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("revenuecat: API error (status %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("[RevenueCat] GetCustomerPurchases raw response: %s\n", string(body))

	var result struct {
		Items []PurchaseItem `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("revenuecat: unmarshal error: %w", err)
	}

	// Translate RevenueCat internal product IDs to store identifiers for credit mapping.
	if len(r.productIDCache) == 0 {
		if err := r.loadProductMapping(ctx); err != nil {
			fmt.Printf("[RevenueCat] Failed to load product mapping: %v\n", err)
		}
	}
	for i := range result.Items {
		if storeID, ok := r.productIDCache[result.Items[i].ProductID]; ok && storeID != "" {
			fmt.Printf("[RevenueCat] Mapped product %s -> %s\n", result.Items[i].ProductID, storeID)
			result.Items[i].ProductID = storeID
		}
	}

	fmt.Printf("[RevenueCat] Parsed purchases: %d\n", len(result.Items))
	return result.Items, nil
}
