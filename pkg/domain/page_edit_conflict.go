package domain

import "fmt"

// PageEditConflictError reports that an editor tried to save an outdated page version.
type PageEditConflictError struct {
	// CurrentRevision is the newest persisted page revision at conflict time.
	CurrentRevision int
}

// Error returns a stable diagnostic message for stale page edits.
func (e *PageEditConflictError) Error() string {
	if e.CurrentRevision <= 0 {
		return "page changed while editing"
	}

	return fmt.Sprintf("page changed while editing; current revision is %d", e.CurrentRevision)
}
