package reviewmodel

import "fmt"

type PlanUsageEnvelope struct {
	EnvelopeVersion      string `json:"envelope_version,omitempty"`
	PlanCode             string `json:"plan_code,omitempty"`
	PlanName             string `json:"plan_name,omitempty"`
	PriceUSD             *int   `json:"price_usd,omitempty"`
	LOCLimitMonth        *int64 `json:"loc_limit_month,omitempty"`
	LOCUsedMonth         *int64 `json:"loc_used_month,omitempty"`
	LOCRemainMonth       *int64 `json:"loc_remaining_month,omitempty"`
	UsagePercent         *int   `json:"usage_percent,omitempty"`
	BillingPeriodStart   string `json:"billing_period_start,omitempty"`
	BillingPeriodEnd     string `json:"billing_period_end,omitempty"`
	ResetAt              string `json:"reset_at,omitempty"`
	ThresholdState       string `json:"threshold_state,omitempty"`
	Blocked              bool   `json:"blocked"`
	TrialReadOnly        bool   `json:"trial_readonly"`
	OperationType        string `json:"operation_type,omitempty"`
	TriggerSource        string `json:"trigger_source,omitempty"`
	OperationBillableLOC *int64 `json:"operation_billable_loc,omitempty"`
	OperationID          string `json:"operation_id,omitempty"`
	IdempotencyKey       string `json:"idempotency_key,omitempty"`
	AccountedAt          string `json:"accounted_at,omitempty"`
	AIExecutionMode      string `json:"ai_execution_mode,omitempty"`
	AIExecutionSource    string `json:"ai_execution_source,omitempty"`
}

type APIErrorPayload struct {
	Error      string             `json:"error,omitempty"`
	ErrorCode  string             `json:"error_code,omitempty"`
	Envelope   *PlanUsageEnvelope `json:"envelope,omitempty"`
	UpgradeURL string             `json:"upgrade_url,omitempty"`
}

// DiffReviewRequest models the POST payload to /api/v1/diff-review.
type DiffReviewRequest struct {
	DiffZipBase64 string `json:"diff_zip_base64"`
	RepoName      string `json:"repo_name"`
}

// DiffReviewResponse models the response from GET /api/v1/diff-review/:id.
type DiffReviewResponse struct {
	Status       string                 `json:"status"`
	Summary      string                 `json:"summary,omitempty"`
	Files        []DiffReviewFileResult `json:"files,omitempty"`
	Message      string                 `json:"message,omitempty"`
	FriendlyName string                 `json:"friendly_name,omitempty"`
	Envelope     *PlanUsageEnvelope     `json:"envelope,omitempty"`
	Quiz         []QuizQuestion         `json:"quiz,omitempty"`
}

// QuizQuestion mirrors one entry of LiveReview's optional "quiz" array — a
// 5-question comprehension quiz generated alongside the summary. Absent
// (nil) for reviews created before this existed, or if LiveReview's LLM
// call didn't produce a well-formed quiz for a given diff.
type QuizQuestion struct {
	Type         string   `json:"type"`
	Question     string   `json:"question"`
	Options      []string `json:"options"`
	CorrectIndex int      `json:"correctIndex"`
	Explanation  string   `json:"explanation,omitempty"`
}

type DiffReviewCreateResponse struct {
	ReviewID     string             `json:"review_id"`
	Status       string             `json:"status"`
	FriendlyName string             `json:"friendly_name,omitempty"`
	UserEmail    string             `json:"user_email,omitempty"`
	Envelope     *PlanUsageEnvelope `json:"envelope,omitempty"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}

type DiffReviewFileResult struct {
	FilePath string              `json:"file_path"`
	Hunks    []DiffReviewHunk    `json:"hunks"`
	Comments []DiffReviewComment `json:"comments"`
}

type DiffReviewHunk struct {
	OldStartLine int    `json:"old_start_line"`
	OldLineCount int    `json:"old_line_count"`
	NewStartLine int    `json:"new_start_line"`
	NewLineCount int    `json:"new_line_count"`
	Content      string `json:"content"`
}

type DiffReviewComment struct {
	Line        int    `json:"line"`
	Content     string `json:"content"`
	Severity    string `json:"severity"`
	Confidence  string `json:"confidence"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
}
