// Command uidev serves the real review UI (from disk, refresh-to-see-edits)
// against captured fixture payloads, so the design of the review page can be
// iterated on with a fully populated review - big diff, real AI comments,
// real blast-radius signal data - without running any reviews.
//
// Fixtures are captured by scripts/capture_design_fixture.sh into
// tools/uidev/fixtures/. Run with:
//
//	go run ./tools/uidev        (or: make design-ui)
//
// then open the printed URL and edit internal/staticserve/static/* - changes
// appear on browser refresh, no rebuild needed.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/HexmosTech/git-lrc/internal/staticserve"
	"github.com/HexmosTech/git-lrc/storage"
)

func main() {
	fixtures := flag.String("fixtures", "tools/uidev/fixtures", "directory holding captured fixture JSON files")
	static := flag.String("static", "internal/staticserve/static", "static assets directory served from disk")
	port := flag.Int("port", 8130, "port to listen on (127.0.0.1 only)")
	flag.Parse()

	staticAbs, err := filepath.Abs(*static)
	if err != nil {
		log.Fatalf("resolving static dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staticAbs, "index.html")); err != nil {
		log.Fatalf("static dir %s has no index.html (run from the repo root?): %v", staticAbs, err)
	}
	reviewFixture := mustRead(filepath.Join(*fixtures, "review-state.json"))
	blastFixture := mustRead(filepath.Join(*fixtures, "blastradius.json"))
	usageFixture := readOr(filepath.Join(*fixtures, "usage-chip.json"), []byte("{}"))

	// Reuse lrc's own disk-serving static handler (no-store caching, .mjs
	// MIME type) by pointing its dev-dir env var at the static tree.
	if err := os.Setenv("LRC_STATIC_DEV_DIR", staticAbs); err != nil {
		log.Fatalf("setting LRC_STATIC_DEV_DIR: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", staticserve.GetStaticHandler()))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		htmlBytes, err := staticserve.ReadFile("index.html")
		if err != nil {
			http.Error(w, "failed to load index.html", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(htmlBytes)
	})

	serveJSON := func(payload []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(payload)
		}
	}
	// No session enforcement: this is a localhost-only design tool and any
	// ?r= value is accepted.
	mux.HandleFunc("/api/review", serveJSON(reviewFixture))
	mux.HandleFunc("/api/blastradius", serveJSON(blastFixture))
	mux.HandleFunc("/api/runtime/usage-chip", serveJSON(usageFixture))

	// The app polls the events proxy once on load; an empty event list keeps
	// the console quiet without simulating a stream.
	mux.HandleFunc("/api/v1/diff-review/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") {
			serveJSON([]byte(`{"events":[]}`))(w, r)
			return
		}
		http.NotFound(w, r)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	// The r param just needs to be non-empty for the app shell to boot.
	fmt.Printf("Design UI: http://%s/?r=design\n", addr)
	fmt.Printf("Static from: %s (edit + refresh, no rebuild)\n", staticAbs)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func mustRead(path string) []byte {
	data, err := storage.ReadFile(path)
	if err != nil {
		log.Fatalf("missing fixture %s (run scripts/capture_design_fixture.sh first): %v", path, err)
	}
	return data
}

func readOr(path string, fallback []byte) []byte {
	data, err := storage.ReadFile(path)
	if err != nil {
		return fallback
	}
	return data
}
