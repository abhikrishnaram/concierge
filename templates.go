package main

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

var pages map[string]*template.Template

// mustParsePages builds one *template.Template per page, each combining the
// shared layout with that page's content block. Call once at startup.
func mustParsePages() {
	pages = map[string]*template.Template{}
	names := []string{"login.html", "webhooks_list.html", "webhooks_form.html", "cron_list.html", "cron_form.html", "runs.html"}
	for _, name := range names {
		pages[name] = template.Must(template.ParseFS(templatesFS, "templates/layout.html", "templates/"+name))
	}
}

func render(w http.ResponseWriter, name string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages[name].ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
