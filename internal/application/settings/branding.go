package settings

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/internal/filetype"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const (
	// MaxBrandLogoBytes is the largest accepted custom brand-logo payload.
	MaxBrandLogoBytes = 2 << 20
)

// BrandLogo contains the persisted instance-wide brand logo.
type BrandLogo struct {
	// ContentType is the validated MIME type returned to browsers.
	ContentType string
	// Data contains the complete logo payload.
	Data []byte
}

// BrandLogo returns the configured custom brand logo.
func (s *Settings) BrandLogo(ctx context.Context) (BrandLogo, error) {
	contentType, data, err := s.repository.BrandLogo(ctx)
	if err != nil {
		return BrandLogo{}, err
	}

	return BrandLogo{ContentType: contentType, Data: data}, nil
}

// SaveBrandLogo validates and persists an instance-wide brand logo.
func (s *Settings) SaveBrandLogo(
	ctx context.Context,
	filename string,
	data []byte,
	actorID int64,
) error {
	if len(data) == 0 {
		return domain.NewValidationError("logo", "Choose a logo image.")
	}
	if len(data) > MaxBrandLogoBytes {
		return domain.NewValidationError("logo", "Logo images must be 2 MiB or smaller.")
	}

	contentType, ok := brandLogoContentType(filename, data)
	if !ok {
		return domain.NewValidationError("logo", "Choose a PNG, JPEG, GIF, WebP, or SVG image.")
	}
	if err := s.repository.SaveBrandLogo(ctx, contentType, data); err != nil {
		return err
	}

	audit.Record(
		ctx, s.logger, s.repository,
		actorID,
		"settings.brand_logo_updated",
		"settings",
		"branding",
		"Updated brand logo",
	)

	return nil
}

// ClearBrandLogo restores the built-in kumbuka.svg brand logo.
func (s *Settings) ClearBrandLogo(ctx context.Context, actorID int64) error {
	if err := s.repository.ClearBrandLogo(ctx); err != nil {
		return err
	}

	audit.Record(
		ctx, s.logger, s.repository,
		actorID,
		"settings.brand_logo_reset",
		"settings",
		"branding",
		"Restored default brand logo",
	)

	return nil
}

// brandLogoContentType returns the canonical MIME type for a supported logo payload.
func brandLogoContentType(filename string, data []byte) (string, bool) {
	if contentType, ok := filetype.DetectImage(data); ok {
		return contentType, true
	}

	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(filename)), ".svg") {
		return "", false
	}
	if !validBrandLogoSVG(data) {
		return "", false
	}

	return "image/svg+xml", true
}

// brandLogoSVGState tracks the structural invariants of one passive SVG document.
type brandLogoSVGState struct {
	// depth is the current XML element nesting depth.
	depth int
	// seenRoot reports whether the single root SVG element has been observed.
	seenRoot bool
}

// validBrandLogoSVG accepts passive SVG documents suitable for image embedding.
func validBrandLogoSVG(data []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	state := brandLogoSVGState{}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return state.seenRoot && state.depth == 0
		}
		if err != nil || !state.accept(token) {
			return false
		}
	}
}

// accept validates one XML token and advances SVG structural state.
func (s *brandLogoSVGState) accept(token xml.Token) bool {
	switch value := token.(type) {
	case xml.Directive:
		return false
	case xml.StartElement:
		return s.acceptStart(value)
	case xml.EndElement:
		s.depth--
		return s.depth >= 0
	default:
		return true
	}
}

// acceptStart validates one element name and its attributes.
func (s *brandLogoSVGState) acceptStart(element xml.StartElement) bool {
	if s.seenRoot && s.depth == 0 {
		return false
	}
	name := strings.ToLower(element.Name.Local)
	if !s.seenRoot {
		if name != "svg" {
			return false
		}
		s.seenRoot = true
	}
	if forbiddenBrandLogoSVGElement(name) || unsafeBrandLogoSVGAttributes(element.Attr) {
		return false
	}
	s.depth++
	return true
}

// forbiddenBrandLogoSVGElement reports whether an SVG element can execute or embed active content.
func forbiddenBrandLogoSVGElement(name string) bool {
	switch name {
	case "script", "foreignobject", "iframe", "object", "embed", "audio", "video":
		return true
	default:
		return false
	}
}

// safeBrandLogoReference reports whether an SVG href is empty, fragment-local, or an embedded image.
func safeBrandLogoReference(value string) bool {
	return value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(strings.ToLower(value), "data:image/")
}

// unsafeBrandLogoSVGAttributes rejects event handlers and external references.
func unsafeBrandLogoSVGAttributes(attributes []xml.Attr) bool {
	for _, attribute := range attributes {
		name := strings.ToLower(attribute.Name.Local)
		if strings.HasPrefix(name, "on") {
			return true
		}
		if name != "href" {
			continue
		}

		value := strings.TrimSpace(attribute.Value)
		if safeBrandLogoReference(value) {
			continue
		}

		return true
	}

	return false
}
