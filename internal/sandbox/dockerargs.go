package sandbox

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
)

// RunOptions are what the harness decides: everything else comes from the
// Compose file.
type RunOptions struct {
	// Name is the container name.
	Name string
	// NetworkName maps the Compose network name to the Docker network the
	// harness created for it.
	NetworkName func(composeName string) string
	// Env is added to the file's environment; it overrides equal keys.
	Env map[string]string
	// Lookup resolves ${VAR} in the file; nil resolves nothing, so defaults
	// apply.
	Lookup func(string) (string, bool)
}

// WorkerRunArgs translates the Worker service into docker run flags, without
// the image and the command. It fails if the service uses a key the harness
// does not translate or a form it cannot represent faithfully.
func (c Compose) WorkerRunArgs(opts RunOptions) ([]string, error) {
	svc, err := c.Worker()
	if err != nil {
		return nil, err
	}
	if len(svc.UnsupportedKeys) > 0 {
		return nil, fmt.Errorf("worker service key %q is not understood by the sandbox harness", svc.UnsupportedKeys[0])
	}
	if len(svc.Ports) > 0 {
		return nil, fmt.Errorf("worker service key %q: the Worker must publish no port", "ports")
	}
	lookup := opts.Lookup
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	args := []string{"--name", opts.Name, "--user", svc.User}
	for _, opt := range svc.SecurityOpt {
		args = append(args, "--security-opt", opt)
	}
	for _, capName := range svc.CapDrop {
		args = append(args, "--cap-drop", capName)
	}
	if svc.ReadOnly {
		args = append(args, "--read-only")
	}
	for _, entry := range svc.Tmpfs {
		args = append(args, "--tmpfs", entry)
	}
	mounts, err := c.mountArgs(svc)
	if err != nil {
		return nil, err
	}
	args = append(args, mounts...)
	args = append(args, envArgs(svc, opts, lookup)...)
	for _, name := range svc.Networks {
		args = append(args, "--network", opts.NetworkName(name))
	}
	args = append(args, "--memory", svc.MemLimit, "--cpus", strconv.FormatFloat(svc.CPUs, 'f', -1, 64),
		"--pids-limit", strconv.Itoa(svc.PidsLimit))
	if svc.Restart != "" {
		args = append(args, "--restart", svc.Restart)
	}
	return args, nil
}

func (c Compose) mountArgs(svc Service) ([]string, error) {
	var args []string
	for _, vol := range svc.Volumes {
		if vol.Type != "bind" {
			return nil, fmt.Errorf("worker volume %s: only bind mounts are translated", vol.Target)
		}
		source := vol.Source
		if !filepath.IsAbs(source) {
			source = filepath.Join(c.Dir, source)
		}
		mode := "rw"
		if vol.ReadOnly {
			mode = "ro"
		}
		args = append(args, "-v", source+":"+vol.Target+":"+mode)
	}
	return args, nil
}

func envArgs(svc Service, opts RunOptions, lookup func(string) (string, bool)) []string {
	merged := map[string]string{}
	for k, v := range svc.Environment {
		merged[k] = Interpolate(v, lookup)
	}
	for k, v := range opts.Env {
		merged[k] = v
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var args []string
	for _, k := range keys {
		args = append(args, "-e", k+"="+merged[k])
	}
	return args
}

// InternalNetworks lists the Compose networks the Worker joins, with whether
// they must be created internal.
func (c Compose) InternalNetworks() (map[string]bool, error) {
	svc, err := c.Worker()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, name := range svc.Networks {
		out[name] = c.Networks[name].Internal
	}
	return out, nil
}
