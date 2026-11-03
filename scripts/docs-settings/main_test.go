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
	{Name: "OUTPRINT_C", Component: "Worker", Type: "`path`", Default: "/x", Tier: settings.Advanced, Description: "Seldom needed."},
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

func TestRenderListsAdvancedSettingsInTheirOwnSectionAtTheEnd(t *testing.T) {
	page := render(sample)
	advanced := strings.Index(page, "\n## Advanced\n")
	if advanced < 0 {
		t.Fatalf("Advanced section missing:\n%s", page)
	}
	if c := strings.Index(page, "### `OUTPRINT_C`"); c < advanced {
		t.Errorf("OUTPRINT_C is not below the Advanced heading:\n%s", page)
	}
	if !strings.Contains(page, "**Read by:** Worker.") {
		t.Errorf("an advanced entry does not name its component:\n%s", page)
	}
}

func TestRenderOmitsTheAdvancedSectionWithoutAdvancedSettings(t *testing.T) {
	if page := render(sample[:2]); strings.Contains(page, "## Advanced") {
		t.Errorf("empty Advanced section:\n%s", page)
	}
}

func TestRenderGroupsByComponent(t *testing.T) {
	page := render(sample)
	if !strings.Contains(page, "## Gateway\n") {
		t.Errorf("Gateway group missing:\n%s", page)
	}
	if strings.Contains(page, "## Worker\n") {
		t.Errorf("a component with only advanced settings got its own group:\n%s", page)
	}
}

func TestRealRegistryRendersEverySetting(t *testing.T) {
	page := render(settings.All())
	for _, s := range settings.All() {
		if !strings.Contains(page, "### `"+s.Name+"`") {
			t.Errorf("%s is missing from the page", s.Name)
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
