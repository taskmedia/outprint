package worker

import "fmt"

// Phase is the pipeline stage that failed.
type Phase string

const (
	PhaseSetup   Phase = "setup"
	PhaseSVG     Phase = "svg"
	PhasePandoc  Phase = "pandoc"
	PhaseFilter  Phase = "filter"
	PhaseTeX     Phase = "tex"
	PhaseTimeout Phase = "timeout"
)

// Kind says what went wrong within a phase.
type Kind string

const (
	KindInvalidJob Kind = "invalid_job"
	KindInternal   Kind = "internal"
	KindToolFailed Kind = "tool_failed"
	KindRawTeX     Kind = "raw_tex"
	KindRawFormat  Kind = "raw_format"
	KindMath       Kind = "math"
	KindImagePath  Kind = "image_path"
	KindTeXError   Kind = "tex_error"
	KindNoOutput   Kind = "no_output"
	KindTimeout    Kind = "timeout"
)

// Error is the only failure the pipeline reports. It names where and what
// failed and which job it was. It never carries tool output: a TeX log can
// quote the Document. The first TeX error line is kept apart for the
// operator log (see FirstTeXError) and is not part of Error().
type Error struct {
	Phase Phase
	Kind  Kind
	JobID string
	// Err is the underlying cause. It must not contain tool output.
	Err error
	// FirstTeXError is the first "!" line of the TeX log, if any.
	FirstTeXError string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("render failed in phase %s (%s), job %s", e.Phase, e.Kind, e.JobID)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }
