package cmd

import (
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/shrsv/dbctx/internal/db"
	"github.com/shrsv/dbctx/internal/ui"
)

//go:embed static
var staticFS embed.FS

var uiCmd = &cobra.Command{
	Use:   "ui [dtx]",
	Short: "Open the database context explorer UI",
	Long:  "Starts a local web server and opens the dbctx explorer in your browser.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtxPath := args[0]
		port, _ := cmd.Flags().GetInt("port")

		store, err := db.OpenStore(dtxPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", dtxPath, err)
		}
		defer store.Close()

		mux := http.NewServeMux()

		// API
		api := ui.NewAPI(store)
		api.Register(mux)

		// Static files
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				data, err := staticFS.ReadFile("static/index.html")
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
			http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
		})

		// Find a port
		var listener net.Listener
		if port > 0 {
			// Explicit port: try exactly that
			listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				return fmt.Errorf("port %d in use", port)
			}
		} else {
			// Default: start at 7777, try next ones if busy
			startPort := 7777
			for p := startPort; p < startPort+50; p++ {
				listener, err = net.Listen("tcp", fmt.Sprintf(":%d", p))
				if err == nil {
					break
				}
			}
			if listener == nil {
				return fmt.Errorf("could not find a free port in range %d-%d", startPort, startPort+49)
			}
		}

		addr := listener.Addr().(*net.TCPAddr)
		url := fmt.Sprintf("http://localhost:%d", addr.Port)
		fmt.Fprintf(os.Stderr, "dbctx ui serving on %s\n", url)
		fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n")

		go openBrowser(url)

		return http.Serve(listener, mux)
	},
}

func init() {
	uiCmd.Flags().IntP("port", "p", 0, "Port to listen on (default: 7777, auto-increments if busy)")
	rootCmd.AddCommand(uiCmd)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
