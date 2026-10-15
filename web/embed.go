// Package web menyediakan filesystem embedded untuk template HTML dan aset statis.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
)

// FS menampung seluruh template dan aset statis yang di-embed ke binary.
//
//go:embed templates/* static/*
var FS embed.FS

var tmpl = template.Must(template.ParseFS(FS, "templates/*.gohtml"))

// Render merender template HTML ke bytes.Buffer terlebih dahulu sebelum ditulis ke ResponseWriter,
// untuk mencegah respons tertulis setengah jika terjadi error eksekusi template.
func Render(w http.ResponseWriter, name string, data any) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("eksekusi template %s: %w", name, err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := buf.WriteTo(w)
	return err
}

// RenderError menampilkan halaman error seragam (mis. 404 atau 500).
func RenderError(w http.ResponseWriter, statusCode int, message string) {
	var buf bytes.Buffer
	data := map[string]any{
		"StatusCode": statusCode,
		"Message":    message,
	}
	if err := tmpl.ExecuteTemplate(&buf, "error.gohtml", data); err != nil {
		http.Error(w, message, statusCode)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = buf.WriteTo(w)
}

// StaticHandler melayani file dari subfolder static/ (css, js, fonts, images).
func StaticHandler() (http.Handler, error) {
	sub, err := fs.Sub(FS, "static")
	if err != nil {
		return nil, fmt.Errorf("sub filesystem static: %w", err)
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, ".woff2") {
			w.Header().Set("Content-Type", "font/woff2")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(path, ".woff") {
			w.Header().Set("Content-Type", "font/woff")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".ico") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}

		fileServer.ServeHTTP(w, r)
	}), nil
}
