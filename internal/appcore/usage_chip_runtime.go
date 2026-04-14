package appcore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/network"
	"github.com/HexmosTech/git-lrc/setup"
	uicfg "github.com/HexmosTech/git-lrc/ui"
)

type runtimeQuotaStatusResponse struct {
	PlanType string                         `json:"plan_type"`
	Envelope *reviewmodel.PlanUsageEnvelope `json:"envelope"`
}

type runtimeSubscriptionCurrentResponse struct {
	PlanType string `json:"plan_type"`
}

type runtimeBillingStatusResponse struct {
	Billing struct {
		CurrentPlanCode  string `json:"current_plan_code"`
		BillingPeriodEnd string `json:"billing_period_end"`
		LOCUsedMonth     int64  `json:"loc_used_month"`
	} `json:"billing"`
	AvailablePlans []struct {
		PlanCode        string `json:"plan_code"`
		MonthlyLOCLimit int64  `json:"monthly_loc_limit"`
	} `json:"available_plans"`
}

type runtimeUpgradeStatusResponse struct {
	Request struct {
		CustomerState string `json:"customer_state"`
	} `json:"request"`
}

type runtimeMyUsageResponse struct {
	Member struct {
		TotalBillableLOC  int64   `json:"total_billable_loc"`
		OperationCount    int64   `json:"operation_count"`
		UsageSharePercent float64 `json:"usage_share_percent"`
	} `json:"member"`
}

type runtimeMembersResponse struct {
	Members []struct {
		ActorEmail       string  `json:"actor_email"`
		ActorKind        string  `json:"actor_kind"`
		TotalBillableLOC int64   `json:"total_billable_loc"`
		UsageSharePct    float64 `json:"usage_share_percent"`
	} `json:"members"`
}

type runtimeUsageError struct {
	Status  int
	Message string
}

func (e *runtimeUsageError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func handleRuntimeUsageChip(w http.ResponseWriter, r *http.Request, config *Config, verbose bool) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	payload := buildRuntimeUsageChipPayload(config, verbose)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(payload)
}

func buildRuntimeUsageChipPayload(config *Config, verbose bool) uicfg.UsageChipResponse {
	payload := uicfg.UsageChipResponse{
		Available:            false,
		UsagePct:             0,
		TopMembers:           make([]uicfg.UsageChipMember, 0),
		CanViewTeamBreakdown: false,
		FetchedAt:            time.Now().UTC().Format(time.RFC3339),
		CloudURL:             setup.CloudAPIURL,
	}

	if config == nil {
		payload.UnavailableReason = "Usage unavailable: runtime config is missing."
		return payload
	}

	apiURL := strings.TrimSpace(config.APIURL)
	if apiURL == "" {
		apiURL = reviewopts.DefaultAPIURL
	}
	if strings.TrimSpace(config.JWT) == "" || strings.TrimSpace(config.OrgID) == "" {
		payload.UnavailableReason = "Not authenticated. Run lrc ui to sign in and select an organization."
		return payload
	}

	client := network.NewReviewAPIClient(15 * time.Second)

	var quotaResp runtimeQuotaStatusResponse
	var billingResp runtimeBillingStatusResponse
	var subscriptionResp runtimeSubscriptionCurrentResponse
	var upgradeResp runtimeUpgradeStatusResponse
	var myUsageResp runtimeMyUsageResponse
	var membersResp runtimeMembersResponse

	var (
		quotaErr, billingErr, subscriptionErr, upgradeErr, myUsageErr, membersErr *runtimeUsageError
		wg                                                                        sync.WaitGroup
	)
	wg.Add(6)
	go func() {
		defer wg.Done()
		quotaErr = fetchRuntimeUsageEndpoint(client, config, apiURL, "/api/v1/quota/status", &quotaResp, verbose)
	}()
	go func() {
		defer wg.Done()
		billingErr = fetchRuntimeUsageEndpoint(client, config, apiURL, "/api/v1/billing/status", &billingResp, verbose)
	}()
	go func() {
		defer wg.Done()
		subscriptionErr = fetchRuntimeUsageEndpoint(client, config, apiURL, "/api/v1/subscriptions/current", &subscriptionResp, verbose)
	}()
	go func() {
		defer wg.Done()
		upgradeErr = fetchRuntimeUsageEndpoint(client, config, apiURL, "/api/v1/billing/upgrade/request-status", &upgradeResp, verbose)
	}()
	go func() {
		defer wg.Done()
		myUsageErr = fetchRuntimeUsageEndpoint(client, config, apiURL, "/api/v1/billing/usage/me", &myUsageResp, verbose)
	}()
	go func() {
		defer wg.Done()
		membersErr = fetchRuntimeUsageEndpoint(client, config, apiURL, "/api/v1/billing/usage/members?limit=3&offset=0", &membersResp, verbose)
	}()
	wg.Wait()

	if quotaErr == nil && quotaResp.Envelope != nil {
		env := quotaResp.Envelope
		payload.PlanCode = strings.TrimSpace(env.PlanCode)
		payload.Blocked = env.Blocked
		payload.ResetAt = strings.TrimSpace(env.ResetAt)
		if env.UsagePercent != nil {
			payload.UsagePct = runtimeUsageClampPercent(*env.UsagePercent)
		}
		if env.LOCUsedMonth != nil {
			payload.LOCUsed = *env.LOCUsedMonth
		}
		if env.LOCLimitMonth != nil {
			payload.LOCLimit = *env.LOCLimitMonth
		}
	}

	if billingErr == nil {
		billingPlanCode := strings.TrimSpace(billingResp.Billing.CurrentPlanCode)
		if billingPlanCode != "" {
			// Treat billing as authoritative when available; quota/subscription can lag.
			payload.PlanCode = billingPlanCode
		} else if payload.PlanCode == "" {
			payload.PlanCode = billingPlanCode
		}

		if billingPeriodEnd := strings.TrimSpace(billingResp.Billing.BillingPeriodEnd); billingPeriodEnd != "" {
			payload.ResetAt = billingPeriodEnd
		} else if payload.ResetAt == "" {
			payload.ResetAt = billingPeriodEnd
		}

		if payload.LOCUsed == 0 {
			payload.LOCUsed = billingResp.Billing.LOCUsedMonth
		}

		planCode := strings.TrimSpace(payload.PlanCode)
		for _, plan := range billingResp.AvailablePlans {
			if strings.TrimSpace(plan.PlanCode) == planCode {
				payload.LOCLimit = plan.MonthlyLOCLimit
				break
			}
		}
	}

	if payload.PlanCode == "" {
		payload.PlanCode = normalizeRuntimeUsagePlanCode(quotaResp.PlanType)
	}
	if payload.PlanCode == "" {
		payload.PlanCode = normalizeRuntimeUsagePlanCode(subscriptionResp.PlanType)
	}
	if payload.LOCLimit == 0 {
		payload.LOCLimit = runtimeUsagePlanLimit(payload.PlanCode)
	}

	if payload.LOCLimit > 0 {
		payload.UsagePct = runtimeUsageClampPercent(int((payload.LOCUsed * 100) / payload.LOCLimit))
	}

	if upgradeErr == nil {
		payload.CustomerState = strings.ToLower(strings.TrimSpace(upgradeResp.Request.CustomerState))
	}

	if myUsageErr == nil {
		payload.MyUsageLOC = myUsageResp.Member.TotalBillableLOC
		payload.MyOperationCount = myUsageResp.Member.OperationCount
		payload.MySharePct = myUsageResp.Member.UsageSharePercent
	}

	if membersErr == nil {
		payload.CanViewTeamBreakdown = true
		for _, member := range membersResp.Members {
			label := strings.TrimSpace(member.ActorEmail)
			if label == "" {
				if strings.EqualFold(strings.TrimSpace(member.ActorKind), "system") {
					label = "System"
				} else {
					label = "Unknown"
				}
			}
			payload.TopMembers = append(payload.TopMembers, uicfg.UsageChipMember{
				Label: label,
				LOC:   member.TotalBillableLOC,
				Share: member.UsageSharePct,
				Kind:  strings.TrimSpace(member.ActorKind),
			})
		}
	} else if membersErr.Status == http.StatusForbidden {
		payload.CanViewTeamBreakdown = false
	}

	payload.Available = (quotaErr == nil || billingErr == nil || subscriptionErr == nil) && runtimeUsageHasSignal(payload)
	if !payload.Available {
		payload.UnavailableReason = runtimeUnavailableReason(quotaErr, billingErr, subscriptionErr, upgradeErr, myUsageErr, membersErr)
	}

	return payload
}

func fetchRuntimeUsageEndpoint(client *network.Client, config *Config, apiURL, path string, out interface{}, verbose bool) *runtimeUsageError {
	status, body, err := runtimeUsageRequest(client, config, apiURL, path, true)
	if err != nil {
		if verbose {
			fmt.Printf("runtime usage endpoint %s failed: %v\n", path, err)
		}
		return &runtimeUsageError{Status: status, Message: err.Error()}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return &runtimeUsageError{Status: status, Message: runtimeUsageErrorMessage(status, body)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &runtimeUsageError{Status: http.StatusBadGateway, Message: fmt.Sprintf("failed to decode %s response: %v", path, err)}
	}
	return nil
}

func runtimeUsageRequest(client *network.Client, config *Config, apiURL, path string, allowRefresh bool) (int, []byte, error) {
	if config == nil {
		return http.StatusUnauthorized, nil, fmt.Errorf("runtime config missing")
	}
	if strings.TrimSpace(config.JWT) == "" || strings.TrimSpace(config.OrgID) == "" {
		return http.StatusUnauthorized, nil, fmt.Errorf("not authenticated")
	}

	fullURL := network.ReviewNormalizedAPIURL(apiURL, path)
	resp, err := network.ReviewForwardJSONWithBearer(client, http.MethodGet, fullURL, nil, config.JWT, config.OrgID)
	if err != nil {
		return http.StatusBadGateway, nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && allowRefresh {
		refreshed, refreshErr := refreshRuntimeUsageTokens(config, apiURL)
		if refreshErr != nil {
			return http.StatusUnauthorized, resp.Body, refreshErr
		}
		if refreshed {
			retryResp, retryErr := network.ReviewForwardJSONWithBearer(client, http.MethodGet, fullURL, nil, config.JWT, config.OrgID)
			if retryErr != nil {
				return http.StatusBadGateway, nil, retryErr
			}
			return retryResp.StatusCode, retryResp.Body, nil
		}
	}
	return resp.StatusCode, resp.Body, nil
}

func refreshRuntimeUsageTokens(config *Config, apiURL string) (bool, error) {
	if config == nil {
		return false, fmt.Errorf("runtime config missing")
	}
	if strings.TrimSpace(config.RefreshToken) == "" {
		return false, fmt.Errorf("refresh token missing")
	}

	newJWT, newRefreshToken, _, _, err := refreshSessionTokens(apiURL, config.RefreshToken)
	if err != nil {
		return false, err
	}

	config.JWT = strings.TrimSpace(newJWT)
	if strings.TrimSpace(newRefreshToken) != "" {
		config.RefreshToken = strings.TrimSpace(newRefreshToken)
	}

	if err := persistConfigUpdates(config.ConfigPath, map[string]string{
		"jwt":           config.JWT,
		"refresh_token": config.RefreshToken,
	}); err != nil {
		return false, err
	}

	return true, nil
}

func runtimeUsageErrorMessage(status int, body []byte) string {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if text := strings.TrimSpace(payload.Error); text != "" {
			return text
		}
		if text := strings.TrimSpace(payload.Message); text != "" {
			return text
		}
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return text
	}
	return fmt.Sprintf("request failed (%d)", status)
}

func runtimeUnavailableReason(errs ...*runtimeUsageError) string {
	for _, usageErr := range errs {
		if usageErr == nil {
			continue
		}
		if usageErr.Status == http.StatusUnauthorized {
			return "Not authenticated. Run lrc ui to sign in and select an organization."
		}
	}
	for _, usageErr := range errs {
		if usageErr == nil {
			continue
		}
		if text := strings.TrimSpace(usageErr.Message); text != "" {
			return text
		}
	}
	return "Usage data unavailable right now."
}

func runtimeUsageClampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func runtimeUsageHasSignal(payload uicfg.UsageChipResponse) bool {
	return strings.TrimSpace(payload.PlanCode) != "" ||
		strings.TrimSpace(payload.ResetAt) != "" ||
		payload.LOCLimit > 0 ||
		payload.LOCUsed > 0 ||
		payload.UsagePct > 0
}

func normalizeRuntimeUsagePlanCode(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return ""
	case "free":
		return "free_30k"
	case "team":
		return "team_32usd"
	default:
		return normalized
	}
}

func runtimeUsagePlanLimit(planCode string) int64 {
	switch normalizeRuntimeUsagePlanCode(planCode) {
	case "free_30k":
		return 30000
	case "team_32usd", "loc_100k":
		return 100000
	case "loc_200k":
		return 200000
	case "loc_400k":
		return 400000
	case "loc_800k":
		return 800000
	case "loc_1600k":
		return 1600000
	case "loc_3200k":
		return 3200000
	default:
		return 0
	}
}
