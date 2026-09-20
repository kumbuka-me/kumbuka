package filetype

import "bytes"

const (
	ImageGIF  = "image/gif"
	ImageJPEG = "image/jpeg"
	ImagePNG  = "image/png"
	ImageWebP = "image/webp"
)

// DetectImage returns the MIME type for the image formats accepted by Kumbuka.
//
// It follows the same signatures used by net/http.DetectContentType for GIF,
// JPEG, PNG, and WebP without coupling application packages to net/http.
func DetectImage(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return ImageGIF, true
	case webP(data):
		return ImageWebP, true
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return ImagePNG, true
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return ImageJPEG, true
	default:
		return "", false
	}
}

// webP reports whether data matches the WebP signature used by MIME sniffing.
func webP(data []byte) bool {
	return len(data) >= 14 &&
		bytes.Equal(data[:4], []byte("RIFF")) &&
		bytes.Equal(data[8:14], []byte("WEBPVP"))
}
