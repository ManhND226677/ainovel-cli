package translation

import "time"

// LiveProgress is an in-memory, presentation-safe view of a chapter currently
// handled by Translation Agent. Durable completion remains in Status; this
// projection exists so a local dashboard can observe work while it is running.
type LiveProgress struct {
	Chapter       int       `json:"chapter"`
	Stage         string    `json:"stage"`
	SourcePreview string    `json:"source_preview,omitempty"`
	Preview       string    `json:"preview,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	Error         string    `json:"error,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProgressObserver is invoked for stage transitions and stream deltas. It must
// return quickly and must not block the translation worker.
type ProgressObserver func(LiveProgress)
