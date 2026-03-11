package ui

const (
	AIConnectorsSectionBegin = "# BEGIN lrc managed ai_connectors"
	AIConnectorsSectionEnd   = "# END lrc managed ai_connectors"
)

type RuntimeConfig struct {
	APIURL        string
	JWT           string
	RefreshJWT    string
	OrgID         string
	UserEmail     string
	UserID        string
	FirstName     string
	LastName      string
	AvatarURL     string
	OrgName       string
	ConfigPath    string
	ConfigErr     string
	ConfigMissing bool
}

type ConnectorRemote struct {
	ID            int64  `json:"id"`
	ProviderName  string `json:"provider_name"`
	ConnectorName string `json:"connector_name"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	SelectedModel string `json:"selected_model"`
	DisplayOrder  int    `json:"display_order"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type AuthRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
