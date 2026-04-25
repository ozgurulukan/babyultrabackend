package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const revenuecatBaseURL = "https://api.revenuecat.com/v2"

type RevenueCatService struct {
	apiKey    string
	projectID string
	client    *http.Client
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
		apiKey:    apiKey,
		projectID: projectID,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
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

	stats := &RevenueStats{
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
	}

	if metrics, ok := result["metrics"].([]interface{}); ok {
		for _, m := range metrics {
			metric, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := metric["name"].(string)
			value, _ := metric["value"].(float64)

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

	url := fmt.Sprintf("%s/projects/%s/customers/%s?expand=non_subscriptions", revenuecatBaseURL, r.projectID, appUserID)
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

	// RevenueCat v2 nests data under "customer"
	// active_entitlements.items[] contains active entitlements
	// non_subscriptions map contains one-time purchases when expanded
	var v2Result struct {
		Customer struct {
			ActiveEntitlements struct {
				Items []struct {
					EntitlementID string `json:"entitlement_id"`
				} `json:"items"`
			} `json:"active_entitlements"`
			NonSubscriptions map[string][]json.RawMessage `json:"non_subscriptions"`
		} `json:"customer"`
	}

	if err := json.Unmarshal(body, &v2Result); err != nil {
		return nil, fmt.Errorf("revenuecat: unmarshal error: %w", err)
	}

	var info CustomerInfo
	// V2: active_entitlements.items indicates active entitlement status
	info.Entitlements.Pro.IsActive = len(v2Result.Customer.ActiveEntitlements.Items) > 0
	for productID := range v2Result.Customer.NonSubscriptions {
		info.NonSubscriptionTransactions = append(info.NonSubscriptionTransactions, struct {
			ProductID string `json:"product_id"`
		}{ProductID: productID})
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

	fmt.Printf("[RevenueCat] Parsed purchases: %d\n", len(result.Items))
	return result.Items, nil
}
