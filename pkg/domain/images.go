package domain

import "time"

// Image contains metadata for one uploaded wiki image.
type Image struct {
	// ID is the stable identifier used in image URLs.
	ID int64 `json:"id"`
	// Filename is the sanitized original image filename.
	Filename string `json:"filename"`
	// ContentType is the validated image MIME type.
	ContentType string `json:"content_type"`
	// SizeBytes is the stored image size in bytes.
	SizeBytes int64 `json:"size_bytes"`
	// UploadedBy is the identifier of the user that uploaded the image.
	UploadedBy int64 `json:"uploaded_by"`
	// Uploader is the display name of the user that uploaded the image.
	Uploader string `json:"uploader"`
	// CreatedAt is the upload timestamp.
	CreatedAt time.Time `json:"created_at"`
	// UsageCount is the number of Markdown references to the image across all pages.
	UsageCount int64 `json:"usage_count"`
}

// ImageData contains the binary payload and response metadata for one image.
type ImageData struct {
	// Filename is the sanitized image filename.
	Filename string
	// ContentType is the validated image MIME type.
	ContentType string
	// Data is the complete stored image payload.
	Data []byte
}
