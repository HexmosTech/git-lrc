package result

// ConvertToJSONData converts HTMLTemplateData into JSONTemplateData.
func ConvertToJSONData(data *HTMLTemplateData) *JSONTemplateData {
	files := make([]JSONFileData, len(data.Files))
	for i, file := range data.Files {
		hunks := make([]JSONHunkData, len(file.Hunks))
		for j, hunk := range file.Hunks {
			lines := make([]JSONLineData, len(hunk.Lines))
			for k, line := range hunk.Lines {
				var comments []JSONCommentData
				if line.IsComment {
					comments = make([]JSONCommentData, len(line.Comments))
					for l, comment := range line.Comments {
						comments[l] = JSONCommentData{
							Severity:    comment.Severity,
							Confidence:  comment.Confidence,
							Type:        comment.Type,
							BadgeClass:  comment.BadgeClass,
							Category:    comment.Category,
							Subcategory: comment.Subcategory,
							Content:     comment.Content,
							HasCategory: comment.HasCategory,
							Line:        comment.Line,
							FilePath:    comment.FilePath,
						}
					}
				}
				lines[k] = JSONLineData{
					OldNum:    line.OldNum,
					NewNum:    line.NewNum,
					Content:   line.Content,
					Class:     line.Class,
					IsComment: line.IsComment,
					Comments:  comments,
				}
			}
			hunks[j] = JSONHunkData{
				Header:      hunk.Header,
				Lines:       lines,
				BlastRadius: hunk.BlastRadius,
			}
		}
		files[i] = JSONFileData{
			ID:           file.ID,
			FilePath:     file.FilePath,
			HasComments:  file.HasComments,
			CommentCount: file.CommentCount,
			Hunks:        hunks,
		}
	}

	return &JSONTemplateData{
		GeneratedTime:      data.GeneratedTime,
		RepositoryPath:     data.RepositoryPath,
		Summary:            data.Summary,
		Status:             data.Status,
		TotalFiles:         data.TotalFiles,
		TotalComments:      data.TotalComments,
		Files:              files,
		HasSummary:         data.HasSummary,
		FriendlyName:       data.FriendlyName,
		Interactive:        data.Interactive,
		IsPostCommitReview: data.IsPostCommitReview,
		InitialMsg:         data.InitialMsg,
		ReviewID:           data.ReviewID,
		APIURL:             data.APIURL,
		APIKey:             data.APIKey,
	}
}
