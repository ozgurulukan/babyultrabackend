package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/ozgurulukan/bubsiebackend/internal/config"
)

type TransformInput struct {
	Model           string                 `json:"model"`
	ImageURL        string                 `json:"image_url"`
	ImageURLs       []string               `json:"image_urls,omitempty"`
	MomImageURL     string                 `json:"mom_image_url,omitempty"`
	BabyImageURL    string                 `json:"baby_image_url,omitempty"`
	DadImageURL     string                 `json:"dad_image_url,omitempty"`
	Prompt          string                 `json:"prompt"`
	NegativePrompt  string                 `json:"negative_prompt,omitempty"`
	Params          map[string]interface{} `json:"params,omitempty"`
}

type TransformOutput struct {
	ResultURL string                 `json:"result_url"`
	Provider  string                 `json:"provider"`
	Model     string                 `json:"model"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type HealthStatus struct {
	Provider string `json:"provider"`
	Active   bool   `json:"active"`
	Message  string `json:"message"`
}

type Provider interface {
	Name() string
	Transform(ctx context.Context, input *TransformInput) (*TransformOutput, error)
	HealthCheck(ctx context.Context) *HealthStatus
}

type Registry struct {
	providers map[string]Provider
}

func isValidKey(key string) bool {
	if len(key) < 8 {
		return false
	}
	lower := strings.ToLower(key)
	placeholders := []string{"sonra", "your_key", "your_", "todo", "placeholder", "xxx", "test", "change_me"}
	for _, p := range placeholders {
		if lower == p || strings.HasPrefix(lower, p) {
			return false
		}
	}
	return true
}

func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		providers: make(map[string]Provider),
	}

	if isValidKey(cfg.FalAIKey) {
		r.Register(NewFalAI(cfg.FalAIKey))
	}
	if isValidKey(cfg.ReplicateKey) {
		r.Register(NewReplicate(cfg.ReplicateKey))
	}
	if isValidKey(cfg.DeepSeekKey) {
		r.Register(NewDeepSeek(cfg.DeepSeekKey))
	}
	if isValidKey(cfg.OpenRouterKey) {
		r.Register(NewOpenRouter(cfg.OpenRouterKey))
	}
	if isValidKey(cfg.GeminiKey) {
		r.Register(NewGemini(cfg.GeminiKey))
	}

	return r
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found or not configured", name)
	}
	return p, nil
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

func (r *Registry) HealthCheckAll(ctx context.Context) []HealthStatus {
	results := make([]HealthStatus, 0, len(r.providers))
	for _, p := range r.providers {
		results = append(results, *p.HealthCheck(ctx))
	}
	return results
}
