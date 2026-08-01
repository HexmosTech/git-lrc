package reviewhtml

import (
	"fmt"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/internal/naming"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/result"
)

type HTMLTemplateData = result.HTMLTemplateData
type HTMLFileData = result.HTMLFileData
type HTMLHunkData = result.HTMLHunkData
type HTMLLineData = result.HTMLLineData
type HTMLCommentData = result.HTMLCommentData

func countTotalComments(files []reviewmodel.DiffReviewFileResult) int {
	total := 0
	for _, file := range files {
		total += len(file.Comments)
	}
	return total
}

func PrepareHTMLData(result *reviewmodel.DiffReviewResponse, interactive bool, isPostCommitReview bool, initialMsg, reviewID, apiURL, apiKey string) *HTMLTemplateData {
	totalComments := countTotalComments(result.Files)

	files := make([]HTMLFileData, len(result.Files))
	for i, file := range result.Files {
		files[i] = PrepareFileData(file)
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

func PrepareFileData(file reviewmodel.DiffReviewFileResult) HTMLFileData {
	fileID := strings.ReplaceAll(file.FilePath, "/", "_")
	hasComments := len(file.Comments) > 0

	commentsByLine := make(map[int][]reviewmodel.DiffReviewComment)
	for _, comment := range file.Comments {
		commentsByLine[comment.Line] = append(commentsByLine[comment.Line], comment)
	}

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

func prepareHunkData(hunk reviewmodel.DiffReviewHunk, commentsByLine map[int][]reviewmodel.DiffReviewComment, filePath string) HTMLHunkData {
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		hunk.OldStartLine, hunk.OldLineCount,
		hunk.NewStartLine, hunk.NewLineCount)

	lines := parseHunkLines(hunk, commentsByLine, filePath)

	return HTMLHunkData{
		Header:      header,
		Lines:       lines,
		BlastRadius: hunk.BlastRadius,
	}
}

func parseHunkLines(hunk reviewmodel.DiffReviewHunk, commentsByLine map[int][]reviewmodel.DiffReviewComment, filePath string) []HTMLLineData {
	contentLines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	var out []HTMLLineData

	for _, line := range contentLines {
		if len(line) == 0 || strings.HasPrefix(line, "@@") {
			continue
		}

		var lineData HTMLLineData

		if strings.HasPrefix(line, "-") {
			lineData = HTMLLineData{OldNum: fmt.Sprintf("%d", oldLine), NewNum: "", Content: line, Class: "diff-del"}
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			lineData = HTMLLineData{OldNum: "", NewNum: fmt.Sprintf("%d", newLine), Content: line, Class: "diff-add"}
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				lineData.IsComment = true
				lineData.Comments = prepareComments(comments, filePath)
			}
			newLine++
		} else {
			lineData = HTMLLineData{OldNum: fmt.Sprintf("%d", oldLine), NewNum: fmt.Sprintf("%d", newLine), Content: " " + line, Class: "diff-context"}
			oldLine++
			newLine++
		}

		out = append(out, lineData)
	}

	return out
}

func prepareComments(comments []reviewmodel.DiffReviewComment, filePath string) []HTMLCommentData {
	out := make([]HTMLCommentData, len(comments))

	for i, comment := range comments {
		severity := strings.ToLower(comment.Severity)
		if severity == "" {
			severity = "info"
		}

		badgeClass := "badge-" + severity
		if severity != "info" && severity != "warning" && severity != "critical" {
			badgeClass = "badge-info"
		}

		out[i] = HTMLCommentData{
			Severity:    strings.ToUpper(severity),
			Confidence:  comment.Confidence,
			Type:        comment.Type,
			BadgeClass:  badgeClass,
			Category:    comment.Category,
			Subcategory: comment.Subcategory,
			Content:     comment.Content,
			HasCategory: comment.Category != "",
			Line:        comment.Line,
			FilePath:    filePath,
		}
	}

	return out
}
