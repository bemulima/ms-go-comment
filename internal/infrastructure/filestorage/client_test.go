package filestorage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

func TestClient_FileLifecycle(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	ownerID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			for field, want := range map[string]string{
				"file_kind": "USER_MEDIA", "owner_id": ownerID.String(), "is_temp": "true", "ttl_minutes": "60",
			} {
				if got := r.FormValue(field); got != want {
					t.Errorf("%s=%q, want %q", field, got, want)
				}
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("form file: %v", err)
			}
			defer file.Close()
			payload, _ := io.ReadAll(file)
			if header.Filename != "image.png" || string(payload) != "png" {
				t.Errorf("filename=%q payload=%q", header.Filename, payload)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": fileID})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/"+fileID.String()+"/activate":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/"+fileID.String()+"/signed-url":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["method"] != "GET" || payload["expires_minutes"] != float64(5) {
				t.Errorf("signed request = %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://signed.example/image"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/files/"+fileID.String():
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	stored, err := client.UploadTemporary(context.Background(), commentuc.TemporaryFileInput{
		OwnerID: ownerID, Filename: "image.png", Data: []byte("png"), TTLMinutes: 60,
	})
	if err != nil || stored.ID != fileID {
		t.Fatalf("upload result=%#v error=%v", stored, err)
	}
	if err := client.Activate(context.Background(), fileID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	url, err := client.SignedGETURL(context.Background(), fileID, 5)
	if err != nil || url != "https://signed.example/image" {
		t.Fatalf("signed url=%q error=%v", url, err)
	}
	if err := client.Delete(context.Background(), fileID); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestClient_RejectsFileStorageFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if err := client.Activate(context.Background(), uuid.New()); err == nil {
		t.Fatal("FileStorage failure was accepted")
	}
}
