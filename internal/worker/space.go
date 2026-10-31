package worker

import "syscall"

// minFreeBytes is the scratch space below which a failed tool is blamed on a
// full job directory.
const minFreeBytes = 1 << 20

// outOfResources reports whether a failed tool died to the system (a signal
// the Worker did not send) or left the job directory full.
func (p *pipeline) outOfResources(res toolResult) bool {
	return res.Signaled || dirFull(p.dir)
}

func dirFull(dir string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return false
	}
	return uint64(st.Bavail)*uint64(st.Bsize) < minFreeBytes
}
