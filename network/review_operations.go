package network

import "net/http"

// ReviewSubmit sends a review submission request to the LiveReview API.
func ReviewSubmit(client *Client, apiURL string, payload any, apiKey string) (*Response, error) {
	return client.DoJSON(http.MethodPost, ReviewSubmitURL(apiURL), payload, "", "", map[string]string{"X-API-Key": apiKey})
}

// ReviewTrackCLIUsage sends CLI usage telemetry to the LiveReview API.
func ReviewTrackCLIUsage(client *Client, apiURL, apiKey string) (*Response, error) {
	return client.DoJSON(http.MethodPost, ReviewCLIUsageURL(apiURL), nil, "", "", map[string]string{"X-API-Key": apiKey})
}

// ReviewPoll fetches review status for a review ID.
func ReviewPoll(client *Client, apiURL, reviewID, apiKey string) (*Response, error) {
	return client.DoJSON(http.MethodGet, ReviewPollURL(apiURL, reviewID), nil, "", "", map[string]string{"X-API-Key": apiKey})
}

// ReviewUploadArtifact POSTs a locally-computed, per-review artifact (e.g. a
// blast-radius report) to LiveReview's generic artifact sync channel. Callers
// should treat failures as non-fatal — this is opportunistic, best-effort
// data (see LiveReview's AGENTS.md "Porting from git-lrc" section).
func ReviewUploadArtifact(client *Client, apiURL, reviewID, artifactType string, payload any, apiKey string) (*Response, error) {
	return client.DoJSON(http.MethodPost, ReviewArtifactURL(apiURL, reviewID, artifactType), payload, "", "", map[string]string{"X-API-Key": apiKey})
}

// ReviewProxyRequest forwards a proxied review request with API key auth.
func ReviewProxyRequest(client *Client, method, apiBase, path, rawQuery string, body []byte, apiKey string) (*Response, error) {
	backendURL := ReviewProxyRequestURL(apiBase, path, rawQuery)
	return client.Do(method, backendURL, body, map[string]string{"X-API-Key": apiKey})
}

// ReviewForwardJSONWithBearer forwards a JSON request with bearer and org headers.
func ReviewForwardJSONWithBearer(client *Client, method, fullURL string, payload []byte, jwt, orgID string) (*Response, error) {
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + jwt,
		"X-Org-Context": orgID,
	}
	return client.Do(method, fullURL, payload, headers)
}
