package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticDemo(t *testing.T) {
	for _, path := range []string{"/", "/style.css", "/app.mjs", "/model.mjs"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || response.Body.Len() == 0 {
				t.Fatalf("resource unavailable: %d", response.Code)
			}
			if !strings.Contains(response.Header().Get("Content-Security-Policy"), "connect-src 'none'") {
				t.Fatal("external connection boundary missing")
			}
		})
	}
}

func TestNoWriteOrUnknownResource(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/send", http.StatusNotFound},
		{http.MethodGet, "/web/", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler().ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.want {
			t.Fatalf("%s %s: got %d", tc.method, tc.path, response.Code)
		}
	}
}
