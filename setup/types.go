package setup

import "encoding/json"

// SetupResult holds the data collected during the setup flow.
type SetupResult struct {
	Email        string
	FirstName    string
	LastName     string
	AvatarURL    string
	UserID       string
	OrgID        string
	OrgName      string
	AccessToken  string
	RefreshToken string
	PlainAPIKey  string
}

// HexmosCallbackData models the ?data= JSON from Hexmos Login redirect.
type HexmosCallbackData struct {
	Result struct {
		JWT  string `json:"jwt"`
		Data struct {
			Email         string `json:"email"`
			Username      string `json:"username"`
			FirstName     string `json:"first_name"`
			LastName      string `json:"last_name"`
			ProfilePicURL string `json:"profilePicUrl"`
		} `json:"data"`
	} `json:"result"`
}

// EnsureCloudUserRequest is the body for POST /api/v1/auth/ensure-cloud-user.
type EnsureCloudUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Source    string `json:"source,omitempty"`
}

// EnsureCloudUserResponse models the response from ensure-cloud-user.
// ID fields use json.Number because the API may return them as integers.
type EnsureCloudUserResponse struct {
	Status string      `json:"status"`
	UserID json.Number `json:"user_id"`
	OrgID  json.Number `json:"org_id"`
	Email  string      `json:"email"`
	User   struct {
		ID        json.Number `json:"id"`
		Email     string      `json:"email"`
		FirstName string      `json:"first_name"`
		LastName  string      `json:"last_name"`
	} `json:"user"`
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    string `json:"expires_at"`
	} `json:"tokens"`
	Organizations []struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	} `json:"organizations"`
}

// CreateAPIKeyRequest is the body for POST /api/v1/orgs/:org_id/api-keys.
type CreateAPIKeyRequest struct {
	Label string `json:"label"`
}

// CreateAPIKeyResponse models the response from creating an API key.
type CreateAPIKeyResponse struct {
	APIKey struct {
		ID    json.Number `json:"id"`
		Label string      `json:"label"`
	} `json:"api_key"`
	PlainKey string `json:"plain_key"`
}

// ValidateKeyRequest is the body for POST /api/v1/aiconnectors/validate-key.
type ValidateKeyRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model,omitempty"`
}

// ValidateKeyResponse models the response from validate-key.
type ValidateKeyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// CreateConnectorRequest is the body for POST /api/v1/aiconnectors.
type CreateConnectorRequest struct {
	ProviderName  string `json:"provider_name"`
	APIKey        string `json:"api_key"`
	ConnectorName string `json:"connector_name"`
	SelectedModel string `json:"selected_model"`
	DisplayOrder  int    `json:"display_order"`
}
