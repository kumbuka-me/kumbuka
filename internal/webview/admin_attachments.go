package webview

import "github.com/kumbuka-me/kumbuka/pkg/domain"

// AdminAttachmentsView contains presentation data for administrator attachment management.
type AdminAttachmentsView struct {
	// Layout contains shared browser presentation.
	Layout

	// Attachments contains stored non-image files and their current reference counts.
	Attachments []domain.Attachment

	// AttachmentQuery is the active filename, uploader, or content-type filter.
	AttachmentQuery string
}
