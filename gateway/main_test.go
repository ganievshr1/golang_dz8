package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayRouting(t *testing.T) {
	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{"GET", "/ping", `{"message":"pong"}`, 200},
		{"GET", "/missing", `{"error":"not found"}`, 404},
		{"POST", "/ping", `{"error":"method not allowed"}`, 405},
	}

	for _, tc := range tests {
		w := httptest.NewRecorder()

		// Эти маршруты не обращаются к сервису.
		newHandler(nil).ServeHTTP(
			w,
			httptest.NewRequest(tc.method, tc.path, nil),
		)

		if w.Code != tc.status ||
			strings.TrimSpace(w.Body.String()) != tc.body {
			t.Fatalf("%d %s", w.Code, w.Body)
		}

		if w.Header().Get("Content-Type") !=
			"application/json; charset=utf-8" {
			t.Fatal("wrong Content-Type")
		}
	}
}
