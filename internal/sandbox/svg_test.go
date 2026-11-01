//go:build sandbox

package sandbox

import (
	"fmt"
	"net"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/svgcorpus"
)

// The SVG attack corpus against the Worker sandbox. The files are posted to
// a real Worker container as bundle images, past the Gateway pre-check, so
// the only defences are librsvg, the SVG phase's limits and the container.
// Every attack must end within the limit with a PDF or a structured error,
// must not show a canary, must not reach a listener on the host, and must
// leave a Worker that renders the next job.

const (
	svgAttackTimeout   = 40 * time.Second
	passwdMarker       = "root:x:0:0"
	svgCanaryInTheWork = "/tmp/svg-canary"
)

// svgErrorKinds are the structured errors an attack may end in.
// A size the converter accepts can still be one TeX cannot place, which is
// a tex_error with no tool output.
var svgErrorKinds = []string{"svg_convert", "timeout", "output_too_large", "resource", "invalid_job", "tex_error"}

// requestListener counts the connections it accepts.
type requestListener struct {
	hostPort string
	hits     atomic.Int64
}

func newRequestListener(t *testing.T) *requestListener {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	l := &requestListener{hostPort: port}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			l.hits.Add(1)
			conn.Close()
		}
	}()
	return l
}

// svgWorld is one Worker with a canary planted in its own file system.
type svgWorld struct {
	w        *liveWorker
	canary   string
	listener *requestListener
	target   svgcorpus.Target
}

func newSVGWorld(t *testing.T, c Compose) *svgWorld {
	t.Helper()
	listener := newRequestListener(t)
	w := startWorker(t, c, workerNetwork(t, c), map[string]string{"RENDERER_TIMEOUT": svgAttackTimeout.String()}, hostAlias...)
	content := "svg-canary-" + randomHex(16)
	if out, err := execIn(t, nil, w.name, "sh", "-c", "printf %s '"+content+"' > "+svgCanaryInTheWork); err != nil {
		t.Fatalf("plant the canary: %v\n%s", err, out)
	}
	return &svgWorld{
		w: w, canary: content, listener: listener,
		target: svgcorpus.Target{CanaryPath: svgCanaryInTheWork, URL: "http://" + hostGateway + ":" + listener.hostPort},
	}
}

func (s *svgWorld) attack(t *testing.T, a svgcorpus.Attack, allowDropped bool) {
	t.Helper()
	b := simpleLetter("Before ![attack](images/img-1.svg) after.\n")
	b.Images = map[string][]byte{"img-1.svg": a.SVG}
	start := time.Now()
	o := s.w.render(b, svgAttackTimeout+requestSlack)
	elapsed := time.Since(start)
	t.Logf("%s: status %d, error %s, transport %q, %s", a.Name, o.Status, o.Error, o.TransportError, elapsed.Round(time.Millisecond))

	if elapsed > svgAttackTimeout+requestSlack {
		t.Errorf("%s ran %s, beyond its limit %s", a.Name, elapsed, svgAttackTimeout)
	}
	for _, leak := range []string{s.canary, passwdMarker} {
		if strings.Contains(o.Text, leak) || strings.Contains(o.Body, leak) || strings.Contains(string(o.Error), leak) {
			t.Errorf("%s: the answer shows %q", a.Name, leak)
		}
	}
	switch {
	case o.TransportError != "" && allowDropped:
		// The attack starved the Worker itself; recovery is the restart.
	case o.TransportError != "":
		t.Errorf("%s: no structured answer: %s", a.Name, o.TransportError)
	case o.Status != 200:
		if e := o.errorBody(); e.JobID == "" || !slices.Contains(svgErrorKinds, e.Kind) {
			t.Errorf("%s: status %d error %s is not a structured SVG error", a.Name, o.Status, o.Error)
		}
	}
	s.w.waitHealthy()
}

func (s *svgWorld) finish(t *testing.T) {
	t.Helper()
	if n := s.listener.hits.Load(); n != 0 {
		t.Errorf("the host listener saw %d connection(s)", n)
	}
	s.w.assertNextJobRenders()
}

// lightChunks is how many Workers share the light attacks. Each attack that
// converts goes on through Pandoc and LuaLaTeX, a few seconds on the
// Worker's one CPU, so the corpus is spread over Workers that run in parallel.
const lightChunks = 4

// TestSVGAttackCorpusEndsInsideTheSandbox runs the corpus. The light attacks
// are dealt to lightChunks Workers, each running its share one after the
// other; each heavy one (memory, time) gets its own Worker. All of them run
// in parallel.
func TestSVGAttackCorpusEndsInsideTheSandbox(t *testing.T) {
	requireDocker(t, sandboxImage())
	c := loadCheckedCompose(t)

	for chunk := range lightChunks {
		t.Run(fmt.Sprintf("light attacks %d", chunk+1), func(t *testing.T) {
			t.Parallel()
			s := newSVGWorld(t, c)
			for i, a := range lightAttacks(s.target) {
				if i%lightChunks == chunk {
					s.attack(t, a, false)
				}
			}
			s.finish(t)
		})
	}
	for _, name := range heavyAttackNames(t) {
		t.Run("heavy "+name, func(t *testing.T) {
			t.Parallel()
			s := newSVGWorld(t, c)
			for _, a := range svgcorpus.Attacks(s.target) {
				if a.Name == name {
					s.attack(t, a, true)
				}
			}
			s.finish(t)
		})
	}
}

func lightAttacks(target svgcorpus.Target) []svgcorpus.Attack {
	return slices.DeleteFunc(svgcorpus.Attacks(target), func(a svgcorpus.Attack) bool { return a.Heavy })
}

func heavyAttackNames(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, a := range svgcorpus.Attacks(svgcorpus.Target{}) {
		if a.Heavy {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		t.Fatal("the corpus has no heavy attacks")
	}
	return names
}

// TestSVGCorpusControlCanaryIsReadableInsideTheContainer shows the canary check can fail:
// the planted file is readable in the Worker's own file system, so a
// converter that did follow file references would have shown it.
func TestSVGCorpusControlCanaryIsReadableInsideTheContainer(t *testing.T) {
	requireDocker(t, sandboxImage())
	s := newSVGWorld(t, loadCheckedCompose(t))
	out, err := execIn(t, nil, s.w.name, "cat", svgCanaryInTheWork)
	if err != nil || out != s.canary {
		t.Fatalf("control: the canary is not readable inside the container: %v %q", err, out)
	}
}
