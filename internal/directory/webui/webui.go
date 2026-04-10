// Package webui embeds the Kaivue React build (ui-v2/dist) into the
// Directory binary so the admin console and integrator portal are served
// without an external web server.
//
// TODO(lead-onprem): mount this handler in the Directory HTTP router at /admin/* and /command/*
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded dist directory as an fs.FS rooted at "dist/".
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("webui: embedded dist sub-tree missing: " + err.Error())
	}
	return sub
}

// IndexHTML returns the contents of the embedded index.html.
func IndexHTML() ([]byte, error) {
	return fs.ReadFile(FS(), "index.html")
}

// Handler returns an http.Handler that serves the embedded SPA.
//
// Static assets are served normally. Any path that does not match a real
// file is rewritten to index.html (SPA fallback). HTML responses carry
// Cache-Control: no-store so browsers always fetch the latest deploy.
//
// mountPrefix is the URL prefix the handler is mounted at (e.g. "/admin").
// It is stripped before looking up files in the embedded FS.
func Handler(mountPrefix string) http.Handler {
	root := FS()
	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the mount prefix so the embedded FS lookup works.
		path := r.URL.Path
		if mountPrefix != "" {
			path = strings.TrimPrefix(path, mountPrefix)
			if path == "" {
				path = "/"
			}
		}

		// Try to open the file. If it exists, serve it directly.
		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		f, err := root.Open(cleanPath)
		if err == nil {
			f.Close()
			// Serve the real file.
			if strings.HasSuffix(cleanPath, ".html") || cleanPath == "index.html" {
				w.Header().Set("Cache-Control", "no-store")
			}
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = r.URL
			r2.URL.Path = path
			fileServer.ServeHTTP(w, r2)
			return
		}

		// SPA fallback: serve index.html for unknown paths.
		index, readErr := IndexHTML()
		if readErr != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(index)
	})
}
