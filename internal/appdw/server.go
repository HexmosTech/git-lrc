package appdw

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/HexmosTech/git-lrc/internal/deepwiki"
	"github.com/HexmosTech/git-lrc/internal/staticserve"
	"github.com/HexmosTech/git-lrc/storage"
)

const defaultDWPort = 8091

// genTask tracks an in-flight (or just-finished) generation for one ref.
type genTask struct {
	status deepwiki.TaskStatus
	done   int
	total  int
	pageID string
	err    string
}

type dwServer struct {
	repoPath      string
	llm           deepwiki.Completer
	provider      string
	model         string
	comprehensive bool

	mu    sync.Mutex
	tasks map[string]*genTask
}

func newDWServer(repoPath string, llm deepwiki.Completer, provider, model string, comprehensive bool) *dwServer {
	return &dwServer{
		repoPath:      repoPath,
		llm:           llm,
		provider:      provider,
		model:         model,
		comprehensive: comprehensive,
		tasks:         map[string]*genTask{},
	}
}

// NewServer exposes a dwServer for reuse (e.g. mounted inside `lrc ui`).
func NewServer(repoPath string, llm deepwiki.Completer, provider, model string, comprehensive bool) *dwServer {
	return newDWServer(repoPath, llm, provider, model, comprehensive)
}

// APIRoutes returns just the /api/dw/* handlers, for mounting under a larger
// mux (the manager UI). The dw SPA fetches these absolute paths.
func (s *dwServer) APIRoutes() http.Handler {
	return s.apiRoutes()
}

func (s *dwServer) apiRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dw/refs", s.handleRefs)
	mux.HandleFunc("/api/dw/wiki", s.handleWiki)
	mux.HandleFunc("/api/dw/generate", s.handleGenerate)
	mux.HandleFunc("/api/dw/status", s.handleStatus)
	return mux
}

// fullRoutes serves the standalone dw app: static assets, the JSON API, and
// the dw.html SPA at "/".
func (s *dwServer) fullRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", staticserve.GetStaticHandler()))
	mux.Handle("/api/dw/", s.apiRoutes())
	mux.HandleFunc("/", ServeDWHTML)
	return mux
}

// ServeDWHTML writes the dw.html SPA, for mounting at /dw in the manager UI.
func ServeDWHTML(w http.ResponseWriter, r *http.Request) {
	html, err := staticserve.ReadFile("dw.html")
	if err != nil {
		http.Error(w, "dw.html not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}

type refsResponse struct {
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
	Commits  []commit `json:"commits"`
	Current  string   `json:"current"`
	Cached   []string `json:"cached"`
}

func (s *dwServer) handleRefs(w http.ResponseWriter, r *http.Request) {
	current := currentRef(s.repoPath)
	refs := listRefs(s.repoPath, current)

	var cached []string
	if entries, err := storage.DeepwikiList(s.repoPath); err == nil {
		for _, e := range entries {
			cached = append(cached, e.Ref)
		}
	}

	writeJSON(w, http.StatusOK, refsResponse{
		Branches: refs.Branches,
		Tags:     refs.Tags,
		Commits:  refs.Commits,
		Current:  refs.Current,
		Cached:   cached,
	})
}

func (s *dwServer) handleWiki(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = currentRef(s.repoPath)
	}
	data, err := storage.DeepwikiLoad(s.repoPath, ref)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":   "not_generated",
			"ref":     ref,
			"cached":  false,
			"message": "No documentation generated for this ref yet.",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}

type generateRequest struct {
	Ref           string `json:"ref"`
	Comprehensive *bool  `json:"comprehensive,omitempty"`
}

func (s *dwServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	if req.Ref == "" {
		req.Ref = currentRef(s.repoPath)
	}

	comprehensive := s.comprehensive
	if req.Comprehensive != nil {
		comprehensive = *req.Comprehensive
	}

	task := s.getOrStart(req.Ref, comprehensive)
	writeJSON(w, http.StatusAccepted, taskStatusJSON(req.Ref, task))
}

func (s *dwServer) getOrStart(ref string, comprehensive bool) *genTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[ref]; ok && t.status != deepwiki.TaskCompleted && t.status != deepwiki.TaskFailed {
		return t
	}
	t := &genTask{status: deepwiki.TaskPending}
	s.tasks[ref] = t
	go s.runGeneration(ref, comprehensive, t)
	return t
}

func (s *dwServer) runGeneration(ref string, comprehensive bool, t *genTask) {
	ctx := context.Background()

	workDir, cleanup, err := materializeRef(s.repoPath, ref)
	if err != nil {
		s.mu.Lock()
		t.status = deepwiki.TaskFailed
		t.err = err.Error()
		s.mu.Unlock()
		return
	}
	defer cleanup()

	wiki, err := deepwiki.Generate(ctx, s.llm, deepwiki.Options{
		RepoPath:      workDir,
		Ref:           ref,
		Comprehensive: comprehensive,
		Language:      "en",
		TreeHash:      treeHash(s.repoPath, ref),
		Provider:      s.provider,
		Model:         s.model,
	}, func(status deepwiki.TaskStatus, done, total int, pageID string) {
		s.mu.Lock()
		t.status = status
		t.done = done
		t.total = total
		t.pageID = pageID
		s.mu.Unlock()
	})

	s.mu.Lock()
	if err != nil {
		t.status = deepwiki.TaskFailed
		t.err = err.Error()
		s.mu.Unlock()
		return
	}
	t.status = deepwiki.TaskCompleted
	t.err = ""
	s.mu.Unlock()

	if data, err := json.MarshalIndent(wiki, "", "  "); err == nil {
		_ = storage.DeepwikiSave(s.repoPath, ref, data)
	}
}

func (s *dwServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = currentRef(s.repoPath)
	}
	s.mu.Lock()
	t := s.tasks[ref]
	s.mu.Unlock()

	if t == nil {
		exists, _ := storage.DeepwikiExists(s.repoPath, ref)
		writeJSON(w, http.StatusOK, map[string]any{
			"ref": ref, "status": "idle", "done": 0, "total": 0,
			"page_id": "", "error": "", "cached": exists,
		})
		return
	}
	exists, _ := storage.DeepwikiExists(s.repoPath, ref)
	writeJSON(w, http.StatusOK, map[string]any{
		"ref": ref, "status": string(t.status), "done": t.done, "total": t.total,
		"page_id": t.pageID, "error": t.err, "cached": exists,
	})
}

func taskStatusJSON(ref string, t *genTask) map[string]any {
	return map[string]any{
		"ref": ref, "status": string(t.status), "done": t.done, "total": t.total,
		"page_id": t.pageID, "error": t.err,
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// serveDW starts the HTTP server and opens the browser, blocking until Ctrl+C.
func serveDW(repoPath string, llm deepwiki.Completer, provider, model string, comprehensive bool, port int, openBrowser bool) error {
	srv := newDWServer(repoPath, llm, provider, model, comprehensive)

	ln, actualPort, err := pickPort(port)
	if err != nil {
		return fmt.Errorf("failed to reserve dw port: %w", err)
	}

	httpServer := &http.Server{Handler: srv.fullRoutes()}
	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("dw server error: %v", err)
		}
	}()

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Printf("\n🌐 DeepWiki available at: %s\n\n", url)

	if openBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = openURL(url)
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}
