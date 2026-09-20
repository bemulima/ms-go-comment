package filestorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	"github.com/google/uuid"
)

const maxErrorBodyBytes = 64 << 10

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c Client) UploadTemporary(ctx context.Context, input commentuc.TemporaryFileInput) (commentuc.StoredFile, error) {
	baseURL, err := c.apiBaseURL()
	if err != nil {
		return commentuc.StoredFile{}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", input.Filename)
	if err != nil {
		return commentuc.StoredFile{}, fmt.Errorf("create file part: %w", err)
	}
	if _, err := part.Write(input.Data); err != nil {
		return commentuc.StoredFile{}, fmt.Errorf("write file part: %w", err)
	}
	fields := map[string]string{
		"file_kind": "USER_MEDIA", "owner_id": input.OwnerID.String(), "is_temp": "true",
		"ttl_minutes": fmt.Sprintf("%d", input.TTLMinutes),
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return commentuc.StoredFile{}, fmt.Errorf("write multipart field %s: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return commentuc.StoredFile{}, fmt.Errorf("close multipart payload: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/files/upload", &body)
	if err != nil {
		return commentuc.StoredFile{}, fmt.Errorf("create upload request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.httpClient().Do(request)
	if err != nil {
		return commentuc.StoredFile{}, fmt.Errorf("upload temporary image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return commentuc.StoredFile{}, responseError("upload temporary image", response)
	}
	var payload struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxErrorBodyBytes)).Decode(&payload); err != nil {
		return commentuc.StoredFile{}, fmt.Errorf("decode upload response: %w", err)
	}
	if payload.ID == uuid.Nil {
		return commentuc.StoredFile{}, fmt.Errorf("upload response does not contain file id")
	}
	return commentuc.StoredFile{ID: payload.ID}, nil
}

func (c Client) Activate(ctx context.Context, fileID uuid.UUID) error {
	return c.noBodyRequest(ctx, http.MethodPost, "/files/"+fileID.String()+"/activate", http.StatusOK, false)
}

func (c Client) SignedGETURL(ctx context.Context, fileID uuid.UUID, expiresMinutes int) (string, error) {
	baseURL, err := c.apiBaseURL()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{"purpose": "comment_attachment", "method": "GET", "expires_minutes": expiresMinutes})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/files/"+fileID.String()+"/signed-url", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create signed URL request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("request signed URL: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", responseError("request signed URL", response)
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxErrorBodyBytes)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode signed URL response: %w", err)
	}
	if strings.TrimSpace(result.URL) == "" {
		return "", fmt.Errorf("signed URL response is empty")
	}
	return result.URL, nil
}

func (c Client) Delete(ctx context.Context, fileID uuid.UUID) error {
	return c.noBodyRequest(ctx, http.MethodDelete, "/files/"+fileID.String(), http.StatusOK, true)
}

func (c Client) noBodyRequest(ctx context.Context, method, path string, success int, notFoundIsSuccess bool) error {
	baseURL, err := c.apiBaseURL()
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create FileStorage request: %w", err)
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return fmt.Errorf("FileStorage request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == success || (notFoundIsSuccess && response.StatusCode == http.StatusNotFound) {
		return nil
	}
	return responseError("FileStorage request", response)
}

func (c Client) apiBaseURL() (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("filestorage base URL is empty")
	}
	if !strings.HasSuffix(baseURL, "/api/v1") {
		baseURL += "/api/v1"
	}
	return baseURL, nil
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func responseError(operation string, response *http.Response) error {
	payload, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
	return fmt.Errorf("%s failed with status %d: %s", operation, response.StatusCode, strings.TrimSpace(string(payload)))
}

var _ commentuc.FileStorage = (*Client)(nil)
