package sandbox

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/settings"
)

const (
	// WorkerUID is the fixed non-root user the Worker image runs as.
	WorkerUID = 10001
	// TokenEnv is the one optional secret of the Worker.
	TokenEnv = settings.EnvWorkerToken

	workDir    = "/work"
	tmpDir     = "/tmp"
	maxTmpSize = 64 << 20
)

var (
	allowedEnv        = []string{settings.EnvListen, settings.EnvWorkDir, TokenEnv}
	requiredMounts    = []string{"/opt/outprint/templates", "/opt/outprint/configurations"}
	allowedSecurity   = []string{"no-new-privileges:true"}
	forbiddenTmpfsOpt = []string{"exec", "suid", "dev"}
)

// CheckWorkerBaseline checks the Worker service against the shipped
// baseline of docs/internals/security.md and returns one message per violation. Each
// message names the Compose key. An empty result means the baseline holds.
func CheckWorkerBaseline(c Compose) []string {
	svc, err := c.Worker()
	if err != nil {
		return []string{err.Error()}
	}
	var v violations
	for _, key := range svc.UnsupportedKeys {
		v.add(key, "is not understood by the sandbox harness; translate it and assert on it before shipping it")
	}
	checkUser(&v, svc)
	checkSecurityOpt(&v, svc)
	checkCapabilities(&v, svc)
	checkFilesystem(&v, svc)
	checkEnvironment(&v, svc)
	checkNetwork(&v, c, svc)
	checkLimits(&v, svc)
	if svc.Restart == "" || svc.Restart == "no" {
		v.add("restart", "must restart the Worker after a timeout, kill or crash; got %q", svc.Restart)
	}
	if !slices.Equal(svc.Command, []string{"worker"}) {
		v.add("command", `must be ["worker"], got %v`, svc.Command)
	}
	return v
}

type violations []string

func (v *violations) add(key, format string, args ...any) {
	*v = append(*v, fmt.Sprintf("%s: "+format, append([]any{key}, args...)...))
}

func parseUser(user string) (uid, gid int, ok bool) {
	u, g, found := strings.Cut(user, ":")
	if !found {
		return 0, 0, false
	}
	uid, errU := strconv.Atoi(u)
	gid, errG := strconv.Atoi(g)
	return uid, gid, errU == nil && errG == nil
}

func checkUser(v *violations, svc Service) {
	uid, gid, ok := parseUser(svc.User)
	switch {
	case !ok:
		v.add("user", `must be a numeric "uid:gid", got %q`, svc.User)
	case uid != WorkerUID || gid != WorkerUID:
		v.add("user", "must be the fixed non-root user %d:%d, got %q", WorkerUID, WorkerUID, svc.User)
	}
}

func checkSecurityOpt(v *violations, svc Service) {
	if !slices.Contains(svc.SecurityOpt, "no-new-privileges:true") {
		v.add("security_opt", "must contain no-new-privileges:true")
	}
	for _, opt := range svc.SecurityOpt {
		if !slices.Contains(allowedSecurity, opt) {
			v.add("security_opt", "%q is not allowed: seccomp, AppArmor and label settings stay at the runtime default", opt)
		}
	}
}

func checkCapabilities(v *violations, svc Service) {
	if !slices.ContainsFunc(svc.CapDrop, func(c string) bool { return strings.EqualFold(c, "ALL") }) {
		v.add("cap_drop", "must contain ALL, got %v", svc.CapDrop)
	}
}

func checkFilesystem(v *violations, svc Service) {
	if !svc.ReadOnly {
		v.add("read_only", "must be true: the root file system is read-only")
	}
	checkTmpfs(v, svc)
	if len(svc.Ports) > 0 {
		v.add("ports", "must be absent: the Worker publishes no port, only the Gateway reaches it")
	}
	checkVolumes(v, svc)
}

func checkTmpfs(v *violations, svc Service) {
	sizes := map[string]int64{}
	for _, entry := range svc.Tmpfs {
		path, opts, _ := strings.Cut(entry, ":")
		o := parseTmpfsOptions(opts)
		for _, bad := range forbiddenTmpfsOpt {
			if _, set := o[bad]; set {
				v.add("tmpfs", "%s must not set %q: tmpfs mounts stay noexec, nosuid, nodev", path, bad)
			}
		}
		size, err := ParseSize(o["size"])
		if err != nil {
			v.add("tmpfs", "%s needs a size, got %q", path, o["size"])
		}
		sizes[path] = size
		if o["uid"] != strconv.Itoa(WorkerUID) || o["gid"] != strconv.Itoa(WorkerUID) {
			v.add("tmpfs", "%s must be owned by uid and gid %d", path, WorkerUID)
		}
		if path == workDir && o["mode"] != "0700" {
			v.add("tmpfs", "%s must have mode=0700, got %q", path, o["mode"])
		}
	}
	if sizes[workDir] != limits.ContainerTmpfsBytes {
		v.add("tmpfs", "%s must be a tmpfs of %d bytes, got %d", workDir, int64(limits.ContainerTmpfsBytes), sizes[workDir])
	}
	if s, ok := sizes[tmpDir]; !ok || s > maxTmpSize {
		v.add("tmpfs", "%s must be a tmpfs of at most %d bytes", tmpDir, int64(maxTmpSize))
	}
	if len(sizes) != 2 {
		v.add("tmpfs", "must be exactly %s and %s, got %d entries", tmpDir, workDir, len(sizes))
	}
}

func parseTmpfsOptions(opts string) map[string]string {
	out := map[string]string{}
	for _, opt := range strings.Split(opts, ",") {
		if opt == "" {
			continue
		}
		key, value, _ := strings.Cut(opt, "=")
		out[key] = value
	}
	return out
}

func checkVolumes(v *violations, svc Service) {
	var targets []string
	for _, vol := range svc.Volumes {
		targets = append(targets, vol.Target)
		if vol.Type != "bind" || !vol.ReadOnly {
			v.add("volumes", "%s must be a read-only bind mount (type bind, read_only true)", vol.Target)
		}
		if !strings.HasPrefix(vol.Source, "../../") || strings.Contains(vol.Source[len("../../"):], "..") {
			v.add("volumes", "%s: source %q must be a package directory of this repository", vol.Target, vol.Source)
		}
	}
	slices.Sort(targets)
	want := slices.Clone(requiredMounts)
	slices.Sort(want)
	if !slices.Equal(targets, want) {
		v.add("volumes", "must mount exactly %v and nothing else (no host mounts, no socket), got %v", requiredMounts, targets)
	}
}

func checkEnvironment(v *violations, svc Service) {
	for key, value := range svc.Environment {
		if !slices.Contains(allowedEnv, key) {
			v.add("environment", "%s is not allowed: the Worker has no secrets except the optional %s", key, TokenEnv)
		}
		if strings.Contains(value, "$") && key != TokenEnv {
			v.add("environment", "%s must be a fixed value, not %q", key, value)
		}
	}
	for _, key := range allowedEnv {
		if _, ok := svc.Environment[key]; !ok {
			v.add("environment", "%s is missing", key)
		}
	}
	if svc.Environment[settings.EnvWorkDir] != workDir {
		v.add("environment", "%s must be %s, got %q", settings.EnvWorkDir, workDir, svc.Environment[settings.EnvWorkDir])
	}
	expanded := Interpolate(svc.Environment[TokenEnv], func(string) (string, bool) { return "", false })
	if expanded != "" {
		v.add("environment", "%s must default to empty, got %q", TokenEnv, expanded)
	}
}

func checkNetwork(v *violations, c Compose, svc Service) {
	if len(svc.Networks) != 1 {
		v.add("networks", "must be exactly one internal network, got %v", svc.Networks)
		return
	}
	name := svc.Networks[0]
	if net, ok := c.Networks[name]; !ok || !net.Internal {
		v.add("networks", "%s must be declared with internal: true so the Worker has no route out", name)
	}
}

func checkLimits(v *violations, svc Service) {
	if mem, err := ParseSize(svc.MemLimit); err != nil || mem != limits.ContainerMemoryBytes {
		v.add("mem_limit", "must be %d bytes (1g), got %q", int64(limits.ContainerMemoryBytes), svc.MemLimit)
	}
	if svc.CPUs != limits.ContainerCPUs {
		v.add("cpus", "must be %d, got %v", limits.ContainerCPUs, svc.CPUs)
	}
	if svc.PidsLimit != limits.ContainerPIDs {
		v.add("pids_limit", "must be %d, got %d", limits.ContainerPIDs, svc.PidsLimit)
	}
}
