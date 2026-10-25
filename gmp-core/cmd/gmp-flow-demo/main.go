// Command gmp-flow-demo serves an isolated, presentation-only operations scene.
// Its fixtures never call the core model, databases, or delivery providers.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"
)

//go:embed web/index.html web/style.css web/app.mjs web/model.mjs
var assets embed.FS

func handler() http.Handler {
	root, err := fs.Sub(assets, "web")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'none'; img-src 'self' data:; style-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "仅提供静态演示", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/", "/index.html", "/style.css", "/app.mjs", "/model.mjs":
			if r.URL.Path == "/app.mjs" || r.URL.Path == "/model.mjs" {
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			}
			files.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "静态演示监听地址（默认仅本机）")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	slog.Info("serving mock-only operations demo", "address", *addr)
	server := &http.Server{Addr: *addr, Handler: handler(), ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		slog.Error("demo server stopped", "error", err)
		os.Exit(1)
	}
}
