package workerserver

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const unknown = "unknown"

// TokenEnv names the optional shared bearer token. It is the one secret the
// Worker may hold.
const TokenEnv = "RENDERER_WORKER_TOKEN"

var secretNameRE = regexp.MustCompile(`(?i)(token|secret|password|passwd|cookie|credential|api_?key|private_?key)`)

// ErrRoot is returned when the Worker would run as UID 0.
var ErrRoot = errors.New("refusing to run as root (UID 0): run the Worker as a fixed non-root user")

// RefuseRoot returns ErrRoot for UID 0.
func RefuseRoot(uid int) error {
	if uid == 0 {
		return ErrRoot
	}
	return nil
}

// Posture is what the Worker can tell about the sandbox it runs in. It is
// logged at start so an operator can see the baseline is in effect. It holds
// no secret value: only the names of environment variables that look like
// one.
type Posture struct {
	UID, GID       int
	NoNewPrivs     string
	ReadOnlyRoot   string
	CapEff         string
	NetInterfaces  string
	SecretEnvNames []string
}

// PostureSource is where the posture is read from; tests replace it.
type PostureSource struct {
	UID, GID int
	ProcDir  string
	Environ  []string
	// Interfaces lists the network interfaces.
	Interfaces func() ([]net.Interface, error)
}

// OSPostureSource reads the running process.
func OSPostureSource() PostureSource {
	return PostureSource{
		UID: os.Getuid(), GID: os.Getgid(), ProcDir: "/proc", Environ: os.Environ(),
		Interfaces: net.Interfaces,
	}
}

// CollectPosture reads what is readable and marks the rest "unknown".
func CollectPosture(src PostureSource) Posture {
	p := Posture{
		UID: src.UID, GID: src.GID,
		NoNewPrivs: unknown, ReadOnlyRoot: unknown, CapEff: unknown, NetInterfaces: unknown,
		SecretEnvNames: secretEnvNames(src.Environ),
	}
	if status, err := os.ReadFile(filepath.Join(src.ProcDir, "self", "status")); err == nil {
		p.NoNewPrivs = statusField(string(status), "NoNewPrivs", func(v string) string { return fmt.Sprint(v == "1") })
		p.CapEff = statusField(string(status), "CapEff", func(v string) string { return v })
	}
	if mounts, err := os.ReadFile(filepath.Join(src.ProcDir, "self", "mountinfo")); err == nil {
		p.ReadOnlyRoot = rootIsReadOnly(string(mounts))
	}
	if src.Interfaces != nil {
		if ifaces, err := src.Interfaces(); err == nil {
			p.NetInterfaces = fmt.Sprint(countNonLoopback(ifaces))
		}
	}
	return p
}

// Attrs are the posture as log attributes.
func (p Posture) Attrs() []any {
	return []any{
		"uid", p.UID, "gid", p.GID, "no_new_privs", p.NoNewPrivs,
		"read_only_root", p.ReadOnlyRoot, "cap_eff", p.CapEff,
		"network_interfaces", p.NetInterfaces, "secret_env_names", strings.Join(p.SecretEnvNames, ","),
	}
}

// Log writes the posture at info level and warns about every part that is
// weaker than the shipped baseline.
func (p Posture) Log(log *slog.Logger) {
	log.Info("worker security posture", p.Attrs()...)
	for _, w := range p.Weaknesses() {
		log.Warn("worker posture is weaker than the shipped baseline", "weakness", w)
	}
}

// Weaknesses lists deviations from the baseline that can be seen from
// inside. An unknown value is not reported: it may just be unreadable.
func (p Posture) Weaknesses() []string {
	var out []string
	if p.NoNewPrivs == "false" {
		out = append(out, "no_new_privs is not set")
	}
	if p.ReadOnlyRoot == "false" {
		out = append(out, "root file system is writable")
	}
	if p.CapEff != unknown && strings.Trim(p.CapEff, "0") != "" {
		out = append(out, "capabilities are not all dropped")
	}
	if len(withoutName(p.SecretEnvNames, TokenEnv)) > 0 {
		out = append(out, "environment holds secret-looking variables")
	}
	return out
}

func withoutName(names []string, drop string) []string {
	var out []string
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}

func statusField(status, name string, convert func(string) string) string {
	for _, line := range strings.Split(status, "\n") {
		if v, ok := strings.CutPrefix(line, name+":"); ok {
			return convert(strings.TrimSpace(v))
		}
	}
	return unknown
}

// rootIsReadOnly looks for the mount at "/" in /proc/self/mountinfo: the
// fifth field is the mount point, the sixth the per-mount options.
func rootIsReadOnly(mountinfo string) string {
	result := unknown
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[4] != "/" {
			continue
		}
		result = fmt.Sprint(!hasOption(fields[5], "rw") && hasOption(fields[5], "ro"))
	}
	return result
}

func hasOption(options, want string) bool {
	for _, o := range strings.Split(options, ",") {
		if o == want {
			return true
		}
	}
	return false
}

func countNonLoopback(ifaces []net.Interface) int {
	n := 0
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback == 0 {
			n++
		}
	}
	return n
}

func secretEnvNames(environ []string) []string {
	var names []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if secretNameRE.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
