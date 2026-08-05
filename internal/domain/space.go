package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MaxAllowedDepth       int16 = 32
	MaxAllowedBodyLength        = 100_000
	MaxAllowedAttachments int16 = 10
	MaxAllowedImageBytes  int64 = 25 * 1024 * 1024
	MaxEditWindowSeconds        = 7 * 24 * 60 * 60
)

var spaceKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

type SpaceStatus int16

const (
	SpaceStatusDisabled SpaceStatus = iota
	SpaceStatusActive
)

func (s SpaceStatus) Valid() bool {
	return s == SpaceStatusDisabled || s == SpaceStatusActive
}

type AccessMode int16

const (
	AccessModeAuthenticated AccessMode = iota + 1
	AccessModeContextGrant
)

func (m AccessMode) Valid() bool {
	return m == AccessModeAuthenticated || m == AccessModeContextGrant
}

type Policy struct {
	AllowImages       bool
	AllowLinks        bool
	MaxDepth          int16
	MaxBodyLength     int
	MaxAttachments    int16
	MaxImageBytes     int64
	EditWindowSeconds int
}

func DefaultPolicy() Policy {
	return Policy{
		AllowLinks:        true,
		MaxDepth:          10,
		MaxBodyLength:     10_000,
		MaxAttachments:    4,
		MaxImageBytes:     5 * 1024 * 1024,
		EditWindowSeconds: 15 * 60,
	}
}

func (p Policy) Validate() error {
	switch {
	case p.MaxDepth < 1 || p.MaxDepth > MaxAllowedDepth:
		return fmt.Errorf("%w: max depth must be between 1 and %d", ErrInvalidPolicy, MaxAllowedDepth)
	case p.MaxBodyLength < 1 || p.MaxBodyLength > MaxAllowedBodyLength:
		return fmt.Errorf("%w: max body length must be between 1 and %d", ErrInvalidPolicy, MaxAllowedBodyLength)
	case p.MaxAttachments < 0 || p.MaxAttachments > MaxAllowedAttachments:
		return fmt.Errorf("%w: max attachments must be between 0 and %d", ErrInvalidPolicy, MaxAllowedAttachments)
	case p.MaxImageBytes < 1 || p.MaxImageBytes > MaxAllowedImageBytes:
		return fmt.Errorf("%w: max image bytes must be between 1 and %d", ErrInvalidPolicy, MaxAllowedImageBytes)
	case p.EditWindowSeconds < 0 || p.EditWindowSeconds > MaxEditWindowSeconds:
		return fmt.Errorf("%w: edit window must be between 0 and %d seconds", ErrInvalidPolicy, MaxEditWindowSeconds)
	default:
		return nil
	}
}

type Space struct {
	ID             uuid.UUID
	Key            string
	Name           string
	Status         SpaceStatus
	AccessMode     AccessMode
	AllowedOrigins []string
	Policy         Policy
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s Space) Validate() error {
	if err := ValidateSpaceKey(s.Key); err != nil {
		return err
	}
	if s.ID == uuid.Nil || s.CreatedBy == uuid.Nil || strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("%w: space identity, creator, and name are required", ErrValidation)
	}
	if !s.Status.Valid() || !s.AccessMode.Valid() {
		return fmt.Errorf("%w: unsupported space status or access mode", ErrValidation)
	}
	for _, origin := range s.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return fmt.Errorf("%w: invalid allowed origin %q", ErrValidation, origin)
		}
	}
	return s.Policy.Validate()
}

func ValidateSpaceKey(key string) error {
	if !spaceKeyPattern.MatchString(key) {
		return ErrInvalidSpaceKey
	}
	return nil
}
