package plugincap

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/sdk"
)

// AttachmentReader must enforce request authorization before reading a range.
// It is supplied explicitly by the composition root; no global media store is
// exposed through the rendering pipeline.
type AttachmentReader interface {
	// ReadAttachment reads attachment.
	ReadAttachment(context.Context, sdk.AttachmentRead) (sdk.Attachment, error)
}

// Attachments exposes an authorized attachment range reader as a plugin capability.
func Attachments(reader AttachmentReader) plugin.Capability {
	return func(ctx context.Context, data json.RawMessage) (any, error) {
		var request sdk.AttachmentRead
		if err := json.Unmarshal(data, &request); err != nil || !validAttachmentRead(request) {
			return nil, errors.New("invalid attachment range")
		}
		attachment, err := reader.ReadAttachment(ctx, request)
		if err != nil {
			return nil, err
		}
		if len(attachment.Data) > request.Length {
			return nil, errors.New("attachment response exceeds range")
		}
		return attachment, nil
	}
}

// validAttachmentRead reports whether an attachment range request is safe and bounded.
func validAttachmentRead(request sdk.AttachmentRead) bool {
	return request.ID > 0 && request.Offset >= 0 && request.Length >= 1 && request.Length <= 1<<20
}
