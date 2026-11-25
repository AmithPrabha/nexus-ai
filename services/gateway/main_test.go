package main

import (
	"net/http/httptest"
	"testing"
)

func TestPublicBoundary(t *testing.T) {
	a := app{origin: "http://localhost:3000"}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/healthz", 200}, {"POST", "/api/v1/demo/sessions", 403}, {"GET", "/api/v1/samples", 401}, {"POST", "/internal/jobs/test/complete", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			a.public().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status || w.Header().Get("X-Request-ID") == "" {
				t.Fatalf("response %v", w.Result())
			}
		})
	}
}
