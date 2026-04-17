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
	client   *http.Client
}

func NewRevenueCatService(apiKey, projectID string) *RevenueCatService {
	return &RevenueCatService{
		apiKey:    apiKey,
		projectID: projectID,
		client:   &http.Client{Timeout: 15 * time.Second},
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
