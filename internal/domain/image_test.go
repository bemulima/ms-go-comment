package domain_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/bemulima/ms-go-comment/internal/domain"
)

func TestInspectImage(t *testing.T) {
	t.Parallel()

	policy := domain.DefaultPolicy()
	policy.AllowImages = true
	tests := []struct {
		name         string
		data         []byte
		filename     string
		policy       domain.Policy
		wantMIME     string
		wantFilename string
		wantErr      error
	}{
		{name: "png signature", data: encodedPNG(t, 2, 3), filename: "photo.jpg", policy: policy, wantMIME: "image/png"},
		{name: "jpeg signature", data: encodedJPEG(t), filename: "photo.jpeg", policy: policy, wantMIME: "image/jpeg"},
		{name: "webp signature", data: decodedBase64(t, "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"), filename: "photo.webp", policy: policy, wantMIME: "image/webp"},
		{name: "windows path is stripped", data: encodedPNG(t, 1, 1), filename: `C:\fakepath\photo.png`, policy: policy, wantMIME: "image/png", wantFilename: "photo.png"},
		{name: "invalid bytes", data: []byte("<svg></svg>"), filename: "photo.png", policy: policy, wantErr: domain.ErrInvalidAttachment},
		{name: "images disabled", data: encodedPNG(t, 1, 1), filename: "photo.png", policy: domain.DefaultPolicy(), wantErr: domain.ErrImagesDisabled},
		{name: "size policy", data: encodedPNG(t, 2, 2), filename: "photo.png", policy: withImageBytes(policy, 1), wantErr: domain.ErrInvalidAttachment},
		{name: "dimension limit", data: encodedPNG(t, 32769, 1), filename: "wide.png", policy: policy, wantErr: domain.ErrInvalidAttachment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := domain.InspectImage(tt.data, tt.filename, tt.policy)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("InspectImage() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || metadata.MIMEType != tt.wantMIME || metadata.SizeBytes != int64(len(tt.data)) ||
				(tt.wantFilename != "" && metadata.OriginalFilename != tt.wantFilename) {
				t.Fatalf("metadata=%#v error=%v", metadata, err)
			}
		})
	}
}

func encodedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

func encodedJPEG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	imageValue := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageValue.Set(0, 0, color.White)
	if err := jpeg.Encode(&buffer, imageValue, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buffer.Bytes()
}

func decodedBase64(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	return data
}

func withImageBytes(policy domain.Policy, size int64) domain.Policy {
	policy.MaxImageBytes = size
	return policy
}
