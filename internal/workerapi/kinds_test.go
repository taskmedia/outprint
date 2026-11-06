package workerapi

import (
	"testing"

	"github.com/taskmedia/outprint/internal/worker"
)

// The Gateway maps these kinds without importing the pipeline, so the
// copies must not drift from it.
func TestPipelineKindsMatchTheWorker(t *testing.T) {
	pairs := map[string]worker.Kind{
		KindTimeout:        worker.KindTimeout,
		KindPagesExceeded:  worker.KindPagesExceeded,
		KindPDFTooLarge:    worker.KindPDFTooLarge,
		KindResource:       worker.KindResource,
		KindOutputTooLarge: worker.KindOutputTooLarge,
	}
	for copied, original := range pairs {
		if copied != string(original) {
			t.Errorf("workerapi kind %q differs from worker kind %q", copied, original)
		}
	}
}
