package main

import (
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/go-chi/chi/v5"
)

var samplePattern = regexp.MustCompile("^[a-z][a-z0-9-]{0,63}$")

func (a *app) preview(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "sample")
	if !samplePattern.MatchString(id) {
		return missing
	}
	var exists bool
	if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT FROM samples WHERE sample_id=$1)", id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return missing
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Security-Policy", "sandbox")
	http.ServeFile(w, r, filepath.Join("/fixtures/invoices", id+".pdf"))
	return nil
}
