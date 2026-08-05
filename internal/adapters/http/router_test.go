package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter_Health(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"service":"ms-go-comment"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestRouter_NotFoundUsesStableError(t *testing.T) {
	response := httptest.NewRecorder()
	NewRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error":"not_found"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
