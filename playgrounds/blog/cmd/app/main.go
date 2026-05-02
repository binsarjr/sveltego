// Command app is the blog playground entry point.
//
// Boots a sveltego server with the codegen-emitted route table and
// hooks, listens on :8080, and renders the markdown-backed blog under
// playgrounds/blog/content/posts. See playgrounds/blog/README.md for a
// feature map and run instructions.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	gen "github.com/binsarjr/sveltego/playgrounds/blog/.gen"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/packages/sveltego/server"
)

func main() {
	shell, err := os.ReadFile("app.html")
	if err != nil {
		log.Fatalf("read app.html: %v", err)
	}
	manifest, err := os.ReadFile("static/_app/.vite/manifest.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("read vite manifest: %v", err)
	}
	s, err := server.New(server.Config{
		Routes:       gen.Routes(),
		Matchers:     params.DefaultMatchers(),
		Shell:        string(shell),
		Hooks:        gen.Hooks(),
		ViteManifest: string(manifest),
		ViteBase:     "/_app",
	})
	if err != nil {
		log.Fatalf("server.New: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/_app/", http.StripPrefix("/_app", server.StaticHandler(kit.StaticConfig{
		Dir:  "static/_app",
		ETag: true,
	})))
	mux.Handle("/", s)
	s.RunInitAsync(context.Background())
	addr := ":8080"
	log.Printf("listening on %s", addr)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(httpSrv.ListenAndServe())
}
