package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taskmedia/outprint/internal/settings"
)

var sample = []settings.Setting{
	{Name: "OUTPRINT_A", Component: "Gateway", Type: "`integer`", Default: "`1`", Tier: settings.Tuning, Description: "First."},
	{Name: "OUTPRINT_B", Component: "Gateway", Type: "`URL`", Tier: settings.MustSet, Description: "Second."},
	{Name: "OUTPRINT_C", Component: "Worker", Type: "`path`", Default: "/x", Tier: settings.Internal, Description: "Hidden."},
}

func TestRenderStatesTheTypeInTheFirstLine(t *testing.T) {
	page := render(sample)
	if !strings.Contains(page, "\n# Settings\n\n**Reference.** ") {
		t.Errorf("the title and the type line are missing:\n%s", page)
	}
}

func TestRenderGivesEverySettingALinkableHeading(t *testing.T) {
	page := render(sample)
	for _, want := range []string{"### `OUTPRINT_A`", "### `OUTPRINT_B`"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing heading %q", want)
		}
	}
}

func TestRenderShowsTypeRequiredAndDefault(t *testing.T) {
	page := render(sample)
	if !strings.Contains(page, "**Type:** `integer`. **Required:** no. **Default:** `1`.") {
		t.Errorf("tuning entry is wrong:\n%s", page)
	}
	if !strings.Contains(page, "**Type:** `URL`. **Required:** yes. **Default:** none.") {
		t.Errorf("must-set entry is wrong:\n%s", page)
	}
}

func TestRenderLeavesOutInternalSettings(t *testing.T) {
	page := render(sample)
	if strings.Contains(page, "OUTPRINT_C") || strings.Contains(page, "Hidden.") {
		t.Errorf("an internal setting was rendered:\n%s", page)
	}
}

func TestRenderGroupsByComponentAndSkipsEmptyGroups(t *testing.T) {
	page := render(sample)
	if !strings.Contains(page, "## Gateway\n") {
		t.Errorf("Gateway group missing:\n%s", page)
	}
	if strings.Contains(page, "## Worker\n") {
		t.Errorf("a group with only internal settings was rendered:\n%s", page)
	}
}

func TestRealRegistryRendersEveryDocumentedSetting(t *testing.T) {
	page := render(settings.All())
	for _, s := range settings.All() {
		has := strings.Contains(page, "### `"+s.Name+"`")
		if s.Tier == settings.Internal && has {
			t.Errorf("%s is internal but rendered", s.Name)
		}
		if s.Tier != settings.Internal && !has {
			t.Errorf("%s is documented but missing", s.Name)
		}
	}
}

func TestCheckFailsWhenTheFileDiffers(t *testing.T) {
	out := filepath.Join(t.TempDir(), "settings.md")
	if err := run(options{out: out}); err != nil {
		t.Fatal(err)
	}
	if err := run(options{out: out, check: true}); err != nil {
		t.Errorf("a fresh page failed the check: %v", err)
	}
	if err := os.WriteFile(out, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(options{out: out, check: true}); err == nil {
		t.Error("a page that differs passed the check")
	}
}

func TestCheckFailsWhenTheFileIsMissing(t *testing.T) {
	out := filepath.Join(t.TempDir(), "absent.md")
	if err := run(options{out: out, check: true}); err == nil {
		t.Error("a missing page passed the check")
	}
}
