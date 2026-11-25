package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDocumentedRoutes(t *testing.T) {
	a := new(app)
	for _, tc := range []struct {
		name   string
		router http.Handler
	}{
		{"openapi.json", a.public()}, {"jobs.openapi.json", a.internal()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile("../../packages/contracts/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Paths map[string]map[string]json.RawMessage
			}
			if err = json.Unmarshal(content, &doc); err != nil {
				t.Fatal(err)
			}
			err = chi.Walk(tc.router.(chi.Routes), func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
				path = strings.TrimSuffix(path, "/")
				op := strings.ToLower(method)
				if _, ok := doc.Paths[path][op]; !ok {
					t.Errorf("Undocumented route %s %s", method, path)
				}
				delete(doc.Paths[path], op)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for path, ops := range doc.Paths {
				for method := range ops {
					t.Errorf("Missing handler for %s %s", method, path)
				}
			}
		})
	}
}
