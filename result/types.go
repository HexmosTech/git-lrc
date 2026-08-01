package result

// HTMLTemplateData contains all data needed for rendering review results.
type HTMLTemplateData struct {
	GeneratedTime string
	// RepositoryPath is the absolute local repository worktree root when available.
	RepositoryPath     string
	Summary            string
	Status             string
	TotalFiles         int
	TotalComments      int
	Files              []HTMLFileData
	HasSummary         bool
	FriendlyName       string
	Interactive        bool
	IsPostCommitReview bool
	InitialMsg         string
	ReviewID           string
	APIURL             string
	APIKey             string
}

type HTMLFileData struct {
	ID           string
	FilePath     string
	HasComments  bool
	CommentCount int
	Hunks        []HTMLHunkData
}

type HTMLHunkData struct {
	Header string
	Lines  []HTMLLineData
	// BlastRadius mirrors reviewmodel.DiffReviewHunk.BlastRadius: nil unless
	// --blast-radius was used.
	BlastRadius *float64
}

type HTMLLineData struct {
	OldNum    string
	NewNum    string
	Content   string
	Class     string
	IsComment bool
	Comments  []HTMLCommentData
}

type HTMLCommentData struct {
	Severity    string
	Confidence  string
	Type        string
	BadgeClass  string
	Category    string
	Subcategory string
	Content     string
	HasCategory bool
	Line        int
	FilePath    string
}

// JSONTemplateData is the structure serialized for the frontend app.
type JSONTemplateData struct {
	GeneratedTime string `json:"GeneratedTime"`
	// RepositoryPath is the absolute local repository worktree root when available.
	RepositoryPath     string         `json:"RepositoryPath,omitempty"`
	Summary            string         `json:"Summary"`
	Status             string         `json:"Status"`
	TotalFiles         int            `json:"TotalFiles"`
	TotalComments      int            `json:"TotalComments"`
	Files              []JSONFileData `json:"Files"`
	HasSummary         bool           `json:"HasSummary"`
	FriendlyName       string         `json:"FriendlyName"`
	Interactive        bool           `json:"Interactive"`
	IsPostCommitReview bool           `json:"IsPostCommitReview"`
	InitialMsg         string         `json:"InitialMsg"`
	ReviewID           string         `json:"ReviewID"`
	APIURL             string         `json:"APIURL"`
	APIKey             string         `json:"APIKey"`
}

type JSONFileData struct {
	ID           string         `json:"ID"`
	FilePath     string         `json:"FilePath"`
	HasComments  bool           `json:"HasComments"`
	CommentCount int            `json:"CommentCount"`
	Hunks        []JSONHunkData `json:"Hunks"`
}

type JSONHunkData struct {
	Header      string         `json:"Header"`
	Lines       []JSONLineData `json:"Lines"`
	BlastRadius *float64       `json:"BlastRadius,omitempty"`
}

type JSONLineData struct {
	OldNum    string            `json:"OldNum"`
	NewNum    string            `json:"NewNum"`
	Content   string            `json:"Content"`
	Class     string            `json:"Class"`
	IsComment bool              `json:"IsComment"`
	Comments  []JSONCommentData `json:"Comments,omitempty"`
}

type JSONCommentData struct {
	Severity    string `json:"Severity"`
	Confidence  string `json:"Confidence"`
	Type        string `json:"Type"`
	BadgeClass  string `json:"BadgeClass"`
	Category    string `json:"Category"`
	Subcategory string `json:"Subcategory"`
	Content     string `json:"Content"`
	HasCategory bool   `json:"HasCategory"`
	Line        int    `json:"Line"`
	FilePath    string `json:"FilePath"`
}
