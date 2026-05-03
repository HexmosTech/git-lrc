package result

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"strings"
)

// RenderPreactHTML renders the Preact-based HTML with embedded JSON data.
func RenderPreactHTML(data *HTMLTemplateData, staticFiles embed.FS) (string, error) {
	jsonData := ConvertToJSONData(data)

	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return "", err
	}

	htmlBytes, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		return "", err
	}

	html := string(htmlBytes)
	html = strings.Replace(html, "{{.JSONData}}", string(jsonBytes), 1)

	if data.FriendlyName != "" {
		html = strings.Replace(html, "<title>LiveReview Results</title>",
			"<title>LiveReview Results — "+data.FriendlyName+"</title>", 1)
	}

	return html, nil
}

// GetStaticHandler returns an HTTP handler for serving static files.
func GetStaticHandler(staticFiles embed.FS) http.Handler {
	_ = mime.AddExtensionType(".mjs", "application/javascript; charset=utf-8")
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(staticFS))
}

// ServeStaticFile serves a specific static file.
func ServeStaticFile(w http.ResponseWriter, filename string, staticFiles embed.FS) error {
	content, err := staticFiles.ReadFile("static/" + filename)
	if err != nil {
		return err
	}

	switch {
	case strings.HasSuffix(filename, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(filename, ".js"), strings.HasSuffix(filename, ".mjs"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(filename, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}

	if _, err := io.Copy(w, bytes.NewReader(content)); err != nil {
		return err
	}
	return nil
}
