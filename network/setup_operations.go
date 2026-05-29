package network

import "net/http"

// SetupEnsureCloudUser submits the ensure-cloud-user setup request.
func SetupEnsureCloudUser(client *Client, cloudBase string, payload any, jwt string) (*Response, error) {
	return client.DoJSON(http.MethodPost, SetupEnsureCloudUserURL(cloudBase), payload, jwt, "", nil)
}

// SetupRefreshTokens submits the auth refresh request.
func SetupRefreshTokens(client *Client, cloudBase string, payload any) (*Response, error) {
	return client.DoJSON(http.MethodPost, SetupAuthRefreshURL(cloudBase), payload, "", "", nil)
}

// SetupCreateAPIKey submits the create API key setup request.
func SetupCreateAPIKey(client *Client, cloudBase, orgID string, payload any, accessToken string) (*Response, error) {
	return client.DoJSON(http.MethodPost, SetupCreateAPIKeyURL(cloudBase, orgID), payload, accessToken, "", nil)
}

// SetupValidateConnectorKey submits the setup key-validation request.
func SetupValidateConnectorKey(client *Client, cloudBase, orgID string, payload any, accessToken string) (*Response, error) {
	return client.DoJSON(http.MethodPost, SetupValidateConnectorKeyURL(cloudBase), payload, accessToken, orgID, nil)
}

// SetupCreateConnector submits the setup create-connector request.
func SetupCreateConnector(client *Client, cloudBase, orgID string, payload any, accessToken string) (*Response, error) {
	return client.DoJSON(http.MethodPost, SetupCreateConnectorURL(cloudBase), payload, accessToken, orgID, nil)
}

// SetupListConnectors retrieves the current connector inventory for setup preflight.
func SetupListConnectors(client *Client, cloudBase, orgID, accessToken string) (*Response, error) {
	return client.DoJSON(http.MethodGet, SetupCreateConnectorURL(cloudBase), nil, accessToken, orgID, nil)
}
