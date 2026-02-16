package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/internal/naming"
)

// Data structures for HTML template rendering
// HTMLTemplateData contains all data needed for the HTML template
// This holds all the template related data structures
type HTMLTemplateData struct {
	GeneratedTime      string
	Summary            string
	Status             string
	TotalFiles         int
	TotalComments      int
	Files              []HTMLFileData
	HasSummary         bool
	FriendlyName       string
	Interactive        bool
	IsPostCommitReview bool // True when reviewing historical commit (--commit mode)
	InitialMsg         string
	ReviewID           string // For polling events
	APIURL             string // For polling events
	APIKey             string // For authenticated API calls
}

// HTMLFileData represents a file for HTML rendering
type HTMLFileData struct {
	ID           string
	FilePath     string
	HasComments  bool
	CommentCount int
	Hunks        []HTMLHunkData
}

// HTMLHunkData represents a hunk for HTML rendering
type HTMLHunkData struct {
	Header string
	Lines  []HTMLLineData
}

// HTMLLineData represents a line in a diff
type HTMLLineData struct {
	OldNum    string
	NewNum    string
	Content   string
	Class     string
	IsComment bool
	Comments  []HTMLCommentData
}

// HTMLCommentData represents a comment for HTML rendering
type HTMLCommentData struct {
	Severity    string
	BadgeClass  string
	Category    string
	Content     string
	HasCategory bool
	Line        int
	FilePath    string
}

// prepareHTMLData converts the API response to template data
func prepareHTMLData(result *diffReviewResponse, interactive bool, isPostCommitReview bool, initialMsg, reviewID, apiURL, apiKey string) *HTMLTemplateData {
	totalComments := countTotalComments(result.Files)

	files := make([]HTMLFileData, len(result.Files))
	for i, file := range result.Files {
		files[i] = prepareFileData(file)
	}

	return &HTMLTemplateData{
		GeneratedTime:      time.Now().Format("2006-01-02 15:04:05 MST"),
		Summary:            result.Summary,
		Status:             result.Status,
		TotalFiles:         len(result.Files),
		TotalComments:      totalComments,
		Files:              files,
		HasSummary:         result.Summary != "",
		FriendlyName:       naming.GenerateFriendlyName(),
		Interactive:        interactive,
		IsPostCommitReview: isPostCommitReview,
		InitialMsg:         initialMsg,
		ReviewID:           reviewID,
		APIURL:             apiURL,
		APIKey:             apiKey,
	}
}

// prepareFileData converts a file result to HTML file data
func prepareFileData(file diffReviewFileResult) HTMLFileData {
	fileID := strings.ReplaceAll(file.FilePath, "/", "_")
	hasComments := len(file.Comments) > 0

	// Create comment lookup map
	commentsByLine := make(map[int][]diffReviewComment)
	for _, comment := range file.Comments {
		commentsByLine[comment.Line] = append(commentsByLine[comment.Line], comment)
	}

	// Process hunks
	hunks := make([]HTMLHunkData, len(file.Hunks))
	for i, hunk := range file.Hunks {
		hunks[i] = prepareHunkData(hunk, commentsByLine, file.FilePath)
	}

	return HTMLFileData{
		ID:           fileID,
		FilePath:     file.FilePath,
		HasComments:  hasComments,
		CommentCount: len(file.Comments),
		Hunks:        hunks,
	}
}

// prepareHunkData converts a hunk to HTML hunk data
func prepareHunkData(hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment, filePath string) HTMLHunkData {
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		hunk.OldStartLine, hunk.OldLineCount,
		hunk.NewStartLine, hunk.NewLineCount)

	lines := parseHunkLines(hunk, commentsByLine, filePath)

	return HTMLHunkData{
		Header: header,
		Lines:  lines,
	}
}

// parseHunkLines parses hunk content into lines with comments
func parseHunkLines(hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment, filePath string) []HTMLLineData {
	contentLines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	var result []HTMLLineData

	for _, line := range contentLines {
		if len(line) == 0 || strings.HasPrefix(line, "@@") {
			continue
		}

		var lineData HTMLLineData

		if strings.HasPrefix(line, "-") {
			lineData = HTMLLineData{
				OldNum:  fmt.Sprintf("%d", oldLine),
				NewNum:  "",
				Content: line,
				Class:   "diff-del",
			}
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			lineData = HTMLLineData{
				OldNum:  "",
				NewNum:  fmt.Sprintf("%d", newLine),
				Content: line,
				Class:   "diff-add",
			}

			// Check for comments on this line
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				lineData.IsComment = true
				lineData.Comments = prepareComments(comments, filePath)
			}

			newLine++
		} else {
			lineData = HTMLLineData{
				OldNum:  fmt.Sprintf("%d", oldLine),
				NewNum:  fmt.Sprintf("%d", newLine),
				Content: " " + line,
				Class:   "diff-context",
			}
			oldLine++
			newLine++
		}

		result = append(result, lineData)
	}

	return result
}

// prepareComments converts comments to HTML comment data
func prepareComments(comments []diffReviewComment, filePath string) []HTMLCommentData {
	result := make([]HTMLCommentData, len(comments))

	for i, comment := range comments {
		severity := strings.ToLower(comment.Severity)
		if severity == "" {
			severity = "info"
		}

		badgeClass := "badge-" + severity
		if severity != "info" && severity != "warning" && severity != "error" && severity != "critical" {
			badgeClass = "badge-info"
		}

		result[i] = HTMLCommentData{
			Severity:    strings.ToUpper(severity),
			BadgeClass:  badgeClass,
			Category:    comment.Category,
			Content:     comment.Content,
			HasCategory: comment.Category != "",
			Line:        comment.Line,
			FilePath:    filePath,
		}
	}

	return result
}

// renderHTMLTemplate renders the HTML using the Preact-based template
func renderHTMLTemplate(data *HTMLTemplateData) (string, error) {
	return renderPreactHTML(data)
}
