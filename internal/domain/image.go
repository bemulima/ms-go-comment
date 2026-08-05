package domain

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path"
	"strings"

	_ "golang.org/x/image/webp"
)

type ImageMetadata struct {
	MIMEType         string
	SizeBytes        int64
	Width            int
	Height           int
	OriginalFilename string
}

func InspectImage(data []byte, originalFilename string, policy Policy) (ImageMetadata, error) {
	if err := policy.Validate(); err != nil {
		return ImageMetadata{}, err
	}
	if !policy.AllowImages || policy.MaxAttachments == 0 {
		return ImageMetadata{}, ErrImagesDisabled
	}
	if len(data) == 0 || int64(len(data)) > policy.MaxImageBytes {
		return ImageMetadata{}, fmt.Errorf("%w: image size is outside policy", ErrInvalidAttachment)
	}
	filename := path.Base(strings.ReplaceAll(strings.TrimSpace(originalFilename), `\`, "/"))
	if filename == "" || filename == "." || strings.ContainsRune(filename, '\x00') || len(filename) > 255 {
		return ImageMetadata{}, fmt.Errorf("%w: original filename is invalid", ErrInvalidAttachment)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("%w: image signature or header is invalid", ErrInvalidAttachment)
	}
	mimeType := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp"}[format]
	if mimeType == "" {
		return ImageMetadata{}, fmt.Errorf("%w: unsupported image format", ErrInvalidAttachment)
	}
	if config.Width < 1 || config.Width > 32768 || config.Height < 1 || config.Height > 32768 {
		return ImageMetadata{}, fmt.Errorf("%w: image dimensions are outside limits", ErrInvalidAttachment)
	}
	return ImageMetadata{
		MIMEType: mimeType, SizeBytes: int64(len(data)), Width: config.Width,
		Height: config.Height, OriginalFilename: filename,
	}, nil
}
