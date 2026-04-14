package ui

// SessionStatusResponse is returned by the connector manager session-status endpoint.
type SessionStatusResponse struct {
	Authenticated  bool                  `json:"authenticated"`
	SessionExpired bool                  `json:"session_expired"`
	MissingConfig  bool                  `json:"missing_config"`
	DisplayName    string                `json:"display_name,omitempty"`
	FirstName      string                `json:"first_name,omitempty"`
	LastName       string                `json:"last_name,omitempty"`
	AvatarURL      string                `json:"avatar_url,omitempty"`
	UserEmail      string                `json:"user_email,omitempty"`
	UserID         string                `json:"user_id,omitempty"`
	OrgID          string                `json:"org_id,omitempty"`
	OrgName        string                `json:"org_name,omitempty"`
	Organizations  []SessionOrganization `json:"organizations,omitempty"`
	APIURL         string                `json:"api_url"`
	Message        string                `json:"message,omitempty"`
}

// SessionOrganization represents an organization option available in lrc ui context switching.
type SessionOrganization struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

// UsageChipMember represents one member usage entry in the top-nav usage popover.
type UsageChipMember struct {
	Label string  `json:"label"`
	LOC   int64   `json:"loc"`
	Share float64 `json:"share"`
	Kind  string  `json:"kind"`
}

// UsageChipResponse is returned by git-lrc local usage endpoints for top-nav rendering.
type UsageChipResponse struct {
	Available            bool              `json:"available"`
	UnavailableReason    string            `json:"unavailable_reason,omitempty"`
	PlanCode             string            `json:"plan_code,omitempty"`
	UsagePct             int               `json:"usage_pct"`
	CustomerState        string            `json:"customer_state,omitempty"`
	Blocked              bool              `json:"blocked"`
	LOCUsed              int64             `json:"loc_used"`
	LOCLimit             int64             `json:"loc_limit"`
	ResetAt              string            `json:"reset_at,omitempty"`
	MyUsageLOC           int64             `json:"my_usage_loc"`
	MyOperationCount     int64             `json:"my_operation_count"`
	MySharePct           float64           `json:"my_share_pct"`
	TopMembers           []UsageChipMember `json:"top_members"`
	CanViewTeamBreakdown bool              `json:"can_view_team_breakdown"`
	CloudURL             string            `json:"cloud_url,omitempty"`
	FetchedAt            string            `json:"fetched_at"`
}
