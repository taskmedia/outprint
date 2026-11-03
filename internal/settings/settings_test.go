package settings_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taskmedia/outprint/internal/admission"
	"github.com/taskmedia/outprint/internal/inject"
	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/settings"
)

func TestRegistryEntriesAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range settings.All() {
		if seen[s.Name] {
			t.Errorf("%s is registered twice", s.Name)
		}
		seen[s.Name] = true
		if !strings.HasPrefix(s.Name, "OUTPRINT_") && !strings.HasPrefix(s.Name, "OUTLINE_") {
			t.Errorf("%s: name must start with OUTPRINT_ or OUTLINE_", s.Name)
		}
		if s.Component == "" || s.Type == "" || s.Description == "" {
			t.Errorf("%s: component, type and description are required", s.Name)
		}
		if s.Tier != settings.MustSet && s.Tier != settings.Tuning && s.Tier != settings.Advanced {
			t.Errorf("%s: unknown tier %q", s.Name, s.Tier)
		}
		if !strings.HasSuffix(s.Description, ".") {
			t.Errorf("%s: description must end with a period", s.Name)
		}
	}
}

func TestAdvancedTierIsExactlyTheDecidedList(t *testing.T) {
	want := []string{
		"OUTPRINT_BUTTON_FALLBACK", "OUTPRINT_BUTTON_FALLBACK_DELAY", "OUTPRINT_BUTTON_METADATA_BLOCK",
		"OUTPRINT_BUTTON_STYLE", "OUTPRINT_DEBUG_TEX_LOG", "OUTPRINT_INJECTOR_GATEWAY_PREFIX",
		"OUTPRINT_INJECTOR_SCRIPT_URL", "OUTPRINT_TEXCACHE_DIR", "OUTPRINT_WORK_DIR",
	}
	if got := namesOf(settings.Advanced); !equal(got, want) {
		t.Errorf("advanced settings\n got %v\nwant %v", got, want)
	}
}

func TestMustSetTierIsWhatTheServicesRefuseToStartWithout(t *testing.T) {
	want := []string{"OUTLINE_URL", "OUTPRINT_INJECTOR_UPSTREAM", "OUTPRINT_WORKER_URL"}
	if got := namesOf(settings.MustSet); !equal(got, want) {
		t.Errorf("must-set settings\n got %v\nwant %v", got, want)
	}
}

func TestDefaultsMatchTheCode(t *testing.T) {
	cases := map[string]string{
		settings.EnvTimeout:             seconds(limits.WallClock),
		settings.EnvMaxPages:            strconv.Itoa(limits.MaxPages),
		settings.EnvMaxPDFBytes:         strconv.Itoa(limits.MaxPDFBytes),
		settings.EnvAdmissionSlots:      strconv.Itoa(admission.Defaults().Slots),
		settings.EnvAdmissionQueueDepth: strconv.Itoa(admission.Defaults().QueueDepth),
		settings.EnvAdmissionPerUser:    strconv.Itoa(admission.Defaults().PerUser),
		settings.EnvAdmissionMaxWait:    seconds(admission.Defaults().MaxWait),
		settings.EnvInjectorListen:      inject.DefaultListen,
		settings.EnvInjectorGatewayPref: inject.DefaultPrefix,
	}
	for name, want := range cases {
		got, ok := lookup(name)
		if !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		if !strings.Contains(got.Default, want) {
			t.Errorf("%s: default %q does not contain the code's %q", name, got.Default, want)
		}
	}
}

var envLiteral = regexp.MustCompile(`^OUT(PRINT|LINE)_[A-Z0-9_]+$`)

// A setting name is spelled once, in the registry. Every other package takes
// it from there, so the registry and the code cannot drift.
func TestNoPackageSpellsASettingName(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			if strings.Contains(filepath.ToSlash(path), "internal/settings/") {
				return nil
			}
			offenders = append(offenders, literalsIn(t, path)...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("setting names spelled outside internal/settings (use the settings constants): %v", offenders)
	}
}

func literalsIn(t *testing.T, path string) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if v, err := strconv.Unquote(lit.Value); err == nil && envLiteral.MatchString(v) {
			found = append(found, path+": "+v)
		}
		return true
	})
	return found
}

func seconds(d time.Duration) string {
	return strconv.Itoa(int(d.Seconds())) + "s"
}

func lookup(name string) (settings.Setting, bool) {
	for _, s := range settings.All() {
		if s.Name == name {
			return s, true
		}
	}
	return settings.Setting{}, false
}

func namesOf(tier settings.Tier) []string {
	var names []string
	for _, s := range settings.All() {
		if s.Tier == tier {
			names = append(names, s.Name)
		}
	}
	sort.Strings(names)
	return names
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
