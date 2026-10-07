package domain

import "time"

// Attachment contains metadata for a stored non-image file.
type Attachment struct {
	// ID identifies attachment.
	ID int64 `json:"id"`
	// Filename is the filename associated with attachment.
	Filename string `json:"filename"`
	// ContentType is the content type associated with attachment.
	ContentType string `json:"content_type"`
	// SizeBytes is the stored attachment size in bytes.
	SizeBytes int64 `json:"size_bytes"`
	// UploadedBy identifies the user who uploaded the attachment.
	UploadedBy int64 `json:"uploaded_by"`
	// Uploader is the display name of the user who uploaded the attachment.
	Uploader string `json:"uploader"`
	// CreatedAt records the created at timestamp for attachment.
	CreatedAt time.Time `json:"created_at"`
	// UsageCount is the number of usage associated with attachment.
	UsageCount int64 `json:"usage_count"`
}

// AttachmentData contains stored attachment bytes.
type AttachmentData struct {
	// Attachment embeds attachment behavior in attachment data.
	Attachment
	// Data contains the data associated with attachment data.
	Data []byte
}
