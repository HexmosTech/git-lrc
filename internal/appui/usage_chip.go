package appui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	setuptpl "github.com/HexmosTech/git-lrc/setup"
	uicfg "github.com/HexmosTech/git-lrc/ui"
)

type usageQuotaStatusResponse struct {
	PlanType string                         `json:"plan_type"`
	Envelope *reviewmodel.PlanUsageEnvelope `json:"envelope"`
}

type usageSubscriptionCurrentResponse struct {
	PlanType string `json:"plan_type"`
}

type usageBillingStatusResponse struct {
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

type usageUpgradeStatusResponse struct {
	Request struct {
		CustomerState string `json:"customer_state"`
	} `json:"request"`
}

type usageMyUsageResponse struct {
	Member struct {
		TotalBillableLOC  int64   `json:"total_billable_loc"`
		OperationCount    int64   `json:"operation_count"`
		UsageSharePercent float64 `json:"usage_share_percent"`
	} `json:"member"`
}

type usageMembersResponse struct {
	Members []struct {
		ActorEmail       string  `json:"actor_email"`
		ActorKind        string  `json:"actor_kind"`
		TotalBillableLOC int64   `json:"total_billable_loc"`
		UsageSharePct    float64 `json:"usage_share_percent"`
	} `json:"members"`
}

type usageAPIError struct {
	Status  int
	Message string
}

func (e *usageAPIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (s *connectorManagerServer) handleUsageChip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	payload := uicfg.UsageChipResponse{
		Available:            false,
		UsagePct:             0,
		TopMembers:           make([]uicfg.UsageChipMember, 0),
		CanViewTeamBreakdown: false,
		CloudURL:             setuptpl.CloudAPIURL,
		FetchedAt:            time.Now().UTC().Format(time.RFC3339),
	}

	var quotaResp usageQuotaStatusResponse
	var billingResp usageBillingStatusResponse
	var subscriptionResp usageSubscriptionCurrentResponse
	var upgradeResp usageUpgradeStatusResponse
	var myUsageResp usageMyUsageResponse
	var membersResp usageMembersResponse

	quotaErr := s.fetchUsageEndpoint("/api/v1/quota/status", &quotaResp)
	billingErr := s.fetchUsageEndpoint("/api/v1/billing/status", &billingResp)
	subscriptionErr := s.fetchUsageEndpoint("/api/v1/subscriptions/current", &subscriptionResp)
	upgradeErr := s.fetchUsageEndpoint("/api/v1/billing/upgrade/request-status", &upgradeResp)
	myUsageErr := s.fetchUsageEndpoint("/api/v1/billing/usage/me", &myUsageResp)
	membersErr := s.fetchUsageEndpoint("/api/v1/billing/usage/members?limit=3&offset=0", &membersResp)

	if quotaErr == nil && quotaResp.Envelope != nil {
		env := quotaResp.Envelope
		payload.PlanCode = strings.TrimSpace(env.PlanCode)
		payload.Blocked = env.Blocked
		payload.ResetAt = strings.TrimSpace(env.ResetAt)
		if env.UsagePercent != nil {
			payload.UsagePct = clampUsagePercent(*env.UsagePercent)
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
		payload.PlanCode = normalizeUsagePlanCode(quotaResp.PlanType)
	}
	if payload.PlanCode == "" {
		payload.PlanCode = normalizeUsagePlanCode(subscriptionResp.PlanType)
	}
	if payload.LOCLimit == 0 {
		payload.LOCLimit = usagePlanLimit(payload.PlanCode)
	}

	if payload.LOCLimit > 0 {
		payload.UsagePct = clampUsagePercent(int((payload.LOCUsed * 100) / payload.LOCLimit))
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

	payload.Available = (quotaErr == nil || billingErr == nil || subscriptionErr == nil) && usagePayloadHasSignal(payload)
	if !payload.Available {
		payload.UnavailableReason = resolveUnavailableReason(quotaErr, billingErr, subscriptionErr, upgradeErr, myUsageErr, membersErr)
	}

	writeJSON(w, http.StatusOK, payload)
}

func (s *connectorManagerServer) fetchUsageEndpoint(path string, out interface{}) *usageAPIError {
	status, body, err := s.proxyJSONRequest(http.MethodGet, path, nil)
	if err != nil {
		return &usageAPIError{Status: http.StatusBadGateway, Message: err.Error()}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return &usageAPIError{Status: status, Message: parseUsageErrorMessage(status, body)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &usageAPIError{Status: http.StatusBadGateway, Message: fmt.Sprintf("failed to decode %s response: %v", path, err)}
	}
	return nil
}

func parseUsageErrorMessage(status int, body []byte) string {
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

func resolveUnavailableReason(errs ...*usageAPIError) string {
	for _, usageErr := range errs {
		if usageErr == nil {
			continue
		}
		if usageErr.Status == http.StatusUnauthorized {
			return "Not authenticated. Open Home and use Re-authenticate."
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

func clampUsagePercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func usagePayloadHasSignal(payload uicfg.UsageChipResponse) bool {
	return strings.TrimSpace(payload.PlanCode) != "" ||
		strings.TrimSpace(payload.ResetAt) != "" ||
		payload.LOCLimit > 0 ||
		payload.LOCUsed > 0 ||
		payload.UsagePct > 0
}

func normalizeUsagePlanCode(raw string) string {
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

func usagePlanLimit(planCode string) int64 {
	switch normalizeUsagePlanCode(planCode) {
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
