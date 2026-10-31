package worker

import (
	"context"
	"path/filepath"
)

// WarmCache renders job once with an empty cache and copies the luaotfload
// cache it produced to dst, which must exist. It is run when the image is
// built, so that jobs find the fonts of the shipped Templates already cached.
func WarmCache(ctx context.Context, job Job, dst string) error {
	job.CacheDir = ""
	var discarded []byte
	return runJob(ctx, job, func(jobDir string) error {
		return copyTree(filepath.Join(jobDir, texmfVarDir), dst, false)
	}, &discarded)
}
