package staticserve

import (
	"embed"
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/HexmosTech/git-lrc/result"
)

//go:embed static/*
var staticFiles embed.FS

type JSONTemplateData = result.JSONTemplateData
type JSONFileData = result.JSONFileData
type JSONHunkData = result.JSONHunkData
type JSONLineData = result.JSONLineData
type JSONCommentData = result.JSONCommentData

// devStaticDir returns the filesystem path set by LRC_STATIC_DEV_DIR.
// When non-empty, static files are served from disk instead of the embedded FS,
// so JS edits are visible on browser refresh without rebuilding the binary.
// Set automatically by `make dev-ui`; not intended for production use.
func devStaticDir() string {
	return os.Getenv("LRC_STATIC_DEV_DIR")
}

// RenderPreactHTML renders the Preact-based HTML with embedded JSON data.
func RenderPreactHTML(data *result.HTMLTemplateData) (string, error) {
	if dir := devStaticDir(); dir != "" {
		jsonData := result.ConvertToJSONData(data)
		jsonBytes, err := json.Marshal(jsonData)
		if err != nil {
			return "", err
		}
		htmlBytes, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			return "", err
		}
		html := strings.Replace(string(htmlBytes), "{{.JSONData}}", string(jsonBytes), 1)
		if data.FriendlyName != "" {
			html = strings.Replace(html, "<title>LiveReview Results</title>",
				"<title>LiveReview Results — "+data.FriendlyName+"</title>", 1)
		}
		return html, nil
	}
	return result.RenderPreactHTML(data, staticFiles)
}

// GetStaticHandler returns an HTTP handler for serving static files.
func GetStaticHandler() http.Handler {
	if dir := devStaticDir(); dir != "" {
		_ = mime.AddExtensionType(".mjs", "application/javascript; charset=utf-8")
		fs := http.FileServer(http.Dir(dir))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			fs.ServeHTTP(w, r)
		})
	}
	return result.GetStaticHandler(staticFiles)
}

// ServeStaticFile serves a specific static file.
func ServeStaticFile(w http.ResponseWriter, r *http.Request, filename string) error {
	return result.ServeStaticFile(w, filename, staticFiles)
}

// ReadFile reads a file from the embedded static directory (or filesystem in dev mode).
func ReadFile(name string) ([]byte, error) {
	if dir := devStaticDir(); dir != "" {
		return os.ReadFile(filepath.Join(dir, name))
	}
	return staticFiles.ReadFile("static/" + name)
}
