package model

type TransformRequest struct {
	Provider   string                 `json:"provider" validate:"required"`
	Model      string                 `json:"model,omitempty"`
	ImageURL   string                 `json:"image_url" validate:"required"`
	ImageURLs  []string               `json:"image_urls,omitempty"`
	MomImageURL  string               `json:"mom_image_url,omitempty"`
	BabyImageURL string               `json:"baby_image_url,omitempty"`
	DadImageURL  string               `json:"dad_image_url,omitempty"`
	Prompt     string                 `json:"prompt" validate:"required"`
	Params     map[string]interface{} `json:"params,omitempty"`
}

type ProviderTestRequest struct {
	Provider string `json:"provider" validate:"required"`
}

type UpdateKeysRequest struct {
	Provider string `json:"provider" validate:"required"`
	Key      string `json:"key" validate:"required"`
}

type ToggleProviderRequest struct {
	Provider string `json:"provider" validate:"required"`
	Enabled  bool   `json:"enabled"`
}

type PlaygroundRequest struct {
	Provider     string                 `json:"provider"`
	Model        string                 `json:"model,omitempty"`
	Prompt       string                 `json:"prompt"`
	ImageURL     string                 `json:"image_url,omitempty"`
	MomImageURL  string                 `json:"mom_image_url,omitempty"`
	BabyImageURL string                 `json:"baby_image_url,omitempty"`
	DadImageURL  string                 `json:"dad_image_url,omitempty"`
	ActionType   string                 `json:"action_type,omitempty"`
	Params       map[string]interface{} `json:"params,omitempty"`
}
