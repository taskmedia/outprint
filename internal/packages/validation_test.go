package packages

import (
	"os"
	"strings"
	"testing"
)

type mutation struct {
	file    string // path below the fixture root
	content string
}

// replaceIn builds the mutation of one fixture file.
func replaceIn(file, old, replacement string) mutation {
	return mutation{file: file, content: strings.Replace(base(file), old, replacement, 1)}
}

func base(file string) string {
	switch file {
	case "templates/plain/manifest.yaml":
		return fixtureManifest
	case "templates/plain/letter.latex":
		return fixtureLetter
	case "configurations/sample/configuration.yaml":
		return fixtureConfiguration
	}
	panic("no base for " + file)
}

func TestLoadErrors(t *testing.T) {
	const (
		manifest = "templates/plain/manifest.yaml"
		letter   = "templates/plain/letter.latex"
		cfgFile  = "configurations/sample/configuration.yaml"
	)
	cases := map[string]struct {
		mutation mutation
		want     []string
	}{
		"unknown template manifest_version": {replaceIn(manifest, "manifest_version: 1", "manifest_version: 2"), []string{"unsupported manifest_version", "2"}},
		"missing template manifest_version": {mutation{manifest, "fields: {}\n"}, []string{"manifest_version is required"}},
		"unknown config manifest_version":   {replaceIn(cfgFile, "manifest_version: 1", "manifest_version: 7"), []string{"unsupported manifest_version"}},
		"config extends":                    {replaceIn(cfgFile, "defaults:", "extends: base\ndefaults:"), []string{"extends is not supported in v1"}},
		"unknown manifest key":              {replaceIn(manifest, "fields:\n", "bogus: 1\nfields:\n"), []string{"unknown key", "bogus"}},
		"unknown config key":                {replaceIn(cfgFile, "defaults:", "bogus: 1\ndefaults:"), []string{"unknown key", "bogus"}},
		"no document types":                 {mutation{manifest, "manifest_version: 1\nfields: {}\n"}, []string{"document_types is required"}},
		"document type template missing":    {replaceIn(manifest, "letter.latex", "nothere.latex"), []string{"template", "nothere.latex"}},
		"document type template escapes":    {replaceIn(manifest, "letter.latex", "../../../etc/passwd"), []string{"escapes its package"}},
		"document type template absolute":   {replaceIn(manifest, "letter.latex", "/etc/passwd"), []string{"relative"}},
		"document type lists unknown field": {replaceIn(manifest, "[name, accent_color, logo, subject]", "[name, accent_color, logo, subject, ghost]"), []string{"ghost", "not declared"}},
		"duplicate field":                   {replaceIn(manifest, "  subject:\n    type: string\n", "  subject:\n    type: string\n  subject:\n    type: string\n"), []string{"duplicate"}},
		"override default breaks pattern":   {replaceIn(manifest, "[name, accent_color, logo, subject]", "[name, {accent_color: {default: 'red}'}}, logo, subject]"), []string{"accent_color", "default"}},
		"override unknown key":              {replaceIn(manifest, "[name, accent_color, logo, subject]", "[name, {accent_color: {bogus: 1}}, logo, subject]"), []string{"bogus"}},
		"color field without pattern":       {replaceIn(manifest, "    pattern: '^[0-9A-Fa-f]{6}$'\n", ""), []string{"accent_color", "typed pattern"}},
		"drift undeclared variable":         {mutation{letter, fixtureLetter + "$ghost$\n"}, []string{"drift", "ghost", "letter.latex:5"}},
		"drift variable not listed":         {replaceIn(manifest, "[name, accent_color, logo, subject]", "[name, accent_color, logo]"), []string{"drift", "subject", "does not list"}},
		"string in colour position":         {mutation{letter, fixtureLetter + `\textcolor{$name$}{x}` + "\n"}, []string{"name", "non-text position", `\textcolor`}},
		"string in length position":         {mutation{letter, fixtureLetter + `\setlength{\parindent}{$name$}` + "\n"}, []string{"name", "non-text position", `\setlength`}},
		"string in filename position":       {mutation{letter, fixtureLetter + `\includegraphics[width=3cm]{$subject$}` + "\n"}, []string{"subject", "non-text position"}},
		"string in option list":             {mutation{letter, fixtureLetter + `\usepackage[$subject$]{xcolor}` + "\n"}, []string{"subject", "non-text position"}},
		"variable in control sequence name": {mutation{letter, fixtureLetter + `\csname$name$` + "\n"}, []string{"control sequence"}},
		"unsupported partial":               {mutation{letter, fixtureLetter + "$header()$\n"}, []string{"letter.latex:5", "not supported"}},
		"input outside package":             {mutation{letter, fixtureLetter + `\input{/etc/passwd}` + "\n"}, []string{"letter.latex:5", `\input`}},
		"input of missing shared file":      {mutation{letter, fixtureLetter + `\input{shared/none.tex}` + "\n"}, []string{"no such file"}},
		"input of unscanned extension":      {mutation{letter, fixtureLetter + `\input{shared/payload.dat}` + "\n"}, []string{"scanned TeX extension"}},
		"input without extension":           {mutation{letter, fixtureLetter + `\input{shared/payload}` + "\n"}, []string{"scanned TeX extension"}},
		"directlua":                         {mutation{letter, fixtureLetter + `\directlua{os.execute("id")}` + "\n"}, []string{`\directlua`, "letter.latex:5"}},
		"write18":                           {mutation{letter, fixtureLetter + `\immediate\write18{id}` + "\n"}, []string{`\write`}},
		"openout":                           {mutation{letter, fixtureLetter + `\openout1=x` + "\n"}, []string{`\openout`}},
		"openin":                            {mutation{letter, fixtureLetter + `\openin1=x` + "\n"}, []string{`\openin`}},
		"hostile shared file":               {mutation{"templates/plain/shared/base.tex", `\directlua{x}`}, []string{"shared/base.tex:1", `\directlua`}},
		"hostile package file":              {mutation{"templates/plain/shared/other.sty", `\catcode` + "`a=12"}, []string{"other.sty", `\catcode`}},
		"hostile unreferenced template":     {mutation{"templates/plain/extra.latex", `\write18{x}`}, []string{"extra.latex", `\write`}},
		"lua file":                          {mutation{"templates/plain/filter.lua", "print(1)"}, []string{"filter.lua", "Lua"}},
		"lfs stub in template":              {mutation{"templates/plain/shared/font.otf", "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 12\n"}, []string{"font.otf", "LFS"}},
		"lfs stub as logo":                  {mutation{"configurations/sample/logo.svg", "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 12\n"}, []string{"logo.svg", "LFS"}},
		"logo escapes package":              {replaceIn(cfgFile, "logo.svg", "../logo.svg"), []string{"logo", "../logo.svg"}},
		"logo absolute":                     {replaceIn(cfgFile, "logo: logo.svg", "logo: /etc/hostname"), []string{"logo"}},
		"logo missing file":                 {replaceIn(cfgFile, "logo: logo.svg", "logo: gone.svg"), []string{"logo", "gone.svg"}},
		"colour default injects tex":        {replaceIn(cfgFile, `accent_color: "2F6F5E"`, `accent_color: 'red}\directlua{os.execute(1)}'`), []string{"accent_color", "pattern"}},
		"colour value injects tex":          {replaceIn(cfgFile, `"2F6F5E"`, `'2F6F5E}\input{/etc/passwd}'`), []string{"accent_color"}},
		"colour default wrong type":         {replaceIn(cfgFile, `accent_color: "2F6F5E"`, "accent_color: [a]"), []string{"accent_color"}},
		"unknown default document type":     {replaceIn(cfgFile, "document_type: letter", "document_type: memo"), []string{"document_type", "memo"}},
		"nested value":                      {replaceIn(cfgFile, "subject: Hello", "subject: {a: b}"), []string{"subject", "value must be"}},
		"invalid config field name":         {replaceIn(cfgFile, "subject: Hello", "'a b': x"), []string{"invalid field name"}},
		"duplicate config key":              {replaceIn(cfgFile, "name: Sample Club", "name: A\n  name: B"), []string{"duplicate"}},
		"broken yaml":                       {mutation{manifest, "fields: [\n"}, []string{"invalid yaml"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			fx.write(tc.mutation.file, tc.mutation.content)
			reg, findings := fx.load()
			if reg != nil {
				t.Fatal("registry must be nil when errors are found")
			}
			if !mentions(findings, SeverityError, tc.want...) {
				t.Fatalf("no error finding containing %q in:\n%s", tc.want, dump(findings))
			}
		})
	}
}

func TestInputOfExistingUnscannedSharedFileIsRejected(t *testing.T) {
	fx := newFixture(t)
	fx.write("templates/plain/shared/payload.dat", `\directlua{os.execute("id")}`)
	fx.write("templates/plain/letter.latex", fixtureLetter+`\input{shared/payload.dat}`+"\n")
	reg, findings := fx.load()
	if reg != nil {
		t.Fatal("registry must be nil when errors are found")
	}
	if !mentions(findings, SeverityError, "payload.dat", "scanned TeX extension") {
		t.Fatalf("no error finding in:\n%s", dump(findings))
	}
}

func TestSymlinkEscapesAreErrors(t *testing.T) {
	fx := newFixture(t)
	fx.write("secret.txt", "secret")
	if err := os.Symlink(fx.path("secret.txt"), fx.path("templates/plain/shared/leak.tex")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fx.path("secret.txt"), fx.path("configurations/sample/leak.svg")); err != nil {
		t.Fatal(err)
	}
	_, findings := fx.load()
	if !mentions(findings, SeverityError, "template/plain", "leak.tex", "symlink escapes") ||
		!mentions(findings, SeverityError, "configuration/sample", "leak.svg", "symlink escapes") {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestSymlinkedLogoOutsidePackageIsRejected(t *testing.T) {
	fx := newFixture(t)
	fx.write("elsewhere.svg", "<svg/>")
	if err := os.Remove(fx.path("configurations/sample/logo.svg")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fx.path("elsewhere.svg"), fx.path("configurations/sample/logo.svg")); err != nil {
		t.Fatal(err)
	}
	_, findings := fx.load()
	if !mentions(findings, SeverityError, "logo.svg", "escapes") {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestSymlinkInsidePackageIsFine(t *testing.T) {
	fx := newFixture(t)
	if err := os.Symlink("logo.svg", fx.path("configurations/sample/alias.svg")); err != nil {
		t.Fatal(err)
	}
	reg, findings := fx.load()
	if reg == nil {
		t.Fatalf("findings:\n%s", dump(findings))
	}
}

func TestWarnings(t *testing.T) {
	cases := map[string]struct {
		file, content string
		want          []string
	}{
		"unused field": {"templates/plain/manifest.yaml",
			strings.Replace(strings.Replace(fixtureManifest, "  subject:\n    type: string\n", "  subject:\n    type: string\n  orphan:\n    type: string\n", 1), "[name, accent_color, logo, subject]", "[name, accent_color, logo, subject, orphan]", 1),
			[]string{"orphan", "never used"}},
		"standard field with other type": {"templates/plain/manifest.yaml",
			strings.Replace(fixtureManifest, "  subject:\n    type: string\n", "  subject:\n    type: text\n", 1),
			[]string{"subject", "Standard Field", "string"}},
		"fixed value not declared": {"configurations/sample/configuration.yaml",
			strings.Replace(fixtureConfiguration, "fields:\n", "fields:\n  extra: 1x\n", 1),
			[]string{"fields.extra", "not declared"}},
		"default shadowed by fixed value": {"configurations/sample/configuration.yaml",
			strings.Replace(fixtureConfiguration, "defaults:\n", "defaults:\n  name: Other\n", 1),
			[]string{"defaults.name", "shadowed"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			fx.write(tc.file, tc.content)
			reg, findings := fx.load()
			if reg == nil || len(Errors(findings)) != 0 {
				t.Fatalf("a warning must not fail the load:\n%s", dump(findings))
			}
			if !mentions(findings, SeverityWarning, tc.want...) {
				t.Fatalf("no warning containing %q in:\n%s", tc.want, dump(findings))
			}
		})
	}
}

func TestStandardFieldLintIsScopedToDocumentType(t *testing.T) {
	fx := newFixture(t)
	// "subject" is a letter field only; in a "memo" document type it is free.
	manifest := strings.Replace(fixtureManifest, "type: string\ndocument", "type: string\ndocument", 1)
	manifest = strings.Replace(manifest, "  subject:\n    type: string\n", "  subject:\n    type: bool\n", 1)
	manifest = strings.Replace(manifest, "  letter:", "  memo:", 1)
	fx.write("templates/plain/manifest.yaml", manifest)
	fx.write("configurations/sample/configuration.yaml", strings.Replace(fixtureConfiguration, "document_type: letter", "document_type: memo", 1))
	_, findings := fx.load()
	if mentions(findings, SeverityWarning, "Standard Field") {
		t.Fatalf("subject is not a standard field of a memo:\n%s", dump(findings))
	}
}

func TestStandardFieldTableMatchesDocs(t *testing.T) {
	doc, err := os.ReadFile("../../docs/reference/standard-fields.md")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for name := range standard.Common {
		names[name] = true
	}
	for name := range standard.Configuration {
		names[name] = true
	}
	for _, fields := range standard.DocumentTypes {
		for name := range fields {
			names[name] = true
		}
	}
	for name := range names {
		if !strings.Contains(string(doc), "`"+name+"`") {
			t.Errorf("standard field %q is not documented in docs/reference/standard-fields.md", name)
		}
	}
	for _, want := range []string{"title", "date", "language", "author", "recipient", "sender", "subject", "opening", "closing", "reference", "enclosures", "cc", "subtitle", "version", "abstract", "toc", "name", "logo", "accent_color", "font"} {
		if !names[want] {
			t.Errorf("Standard Field %q missing from the table", want)
		}
	}
}

// Security layer 5, end to end through the loader: every hostile snippet
// placed in a Template file fails the load. There is no skip path here.
func TestHostileTemplateSnippetsFailTheLoad(t *testing.T) {
	for name, snippet := range hostileSnippets {
		for _, target := range []string{"templates/plain/letter.latex", "templates/plain/shared/base.tex"} {
			t.Run(name+" in "+target, func(t *testing.T) {
				fx := newFixture(t)
				content := fixtureLetter
				if strings.HasSuffix(target, "base.tex") {
					content = `\newcommand{\sample}{x}`
				}
				fx.write(target, content+"\n"+snippet+"\n")
				reg, findings := fx.load()
				if reg != nil || len(Errors(findings)) == 0 {
					t.Fatalf("hostile snippet %q was accepted:\n%s", snippet, dump(findings))
				}
			})
		}
	}
}

// Hostile Configuration values must not get past the typed patterns.
func TestHostileConfigurationValuesFailTheLoad(t *testing.T) {
	hostile := map[string][]string{
		"accent_color": {`red}\directlua{os.execute("id")}`, `FF0000}\input{/etc/passwd}`, "FF00\n00", `\x`, `{}`, `ffffff%`, `$(id)`, ""},
		"logo":         {"../logo.svg", "/etc/passwd.svg", `logo.svg}\write18{id}`, "a/../b.svg", `logo\.svg`, "logo.svg\n", "|id.svg", "~/logo.svg"},
	}
	for field, values := range hostile {
		for _, v := range values {
			for _, section := range []string{"fields", "defaults"} {
				fx := newFixture(t)
				cfg := strings.Replace(fixtureConfiguration, section+":\n", section+":\n  "+field+": "+yamlQuote(v)+"\n", 1)
				if section == "fields" {
					cfg = strings.Replace(cfg, "  "+field+": "+yamlQuote(v)+"\n", "  "+field+": "+yamlQuote(v)+"\n", 1)
					cfg = removeLine(cfg, "  accent_color: \"2F6F5E\"", field == "accent_color")
					cfg = removeLine(cfg, "  logo: logo.svg", field == "logo")
				}
				fx.write("configurations/sample/configuration.yaml", cfg)
				reg, findings := fx.load()
				if reg != nil || len(Errors(findings)) == 0 {
					t.Errorf("%s.%s = %q was accepted:\n%s", section, field, v, dump(findings))
				}
			}
		}
	}
}

func yamlQuote(s string) string {
	return "'" + strings.NewReplacer("'", "''", "\n", "\\n").Replace(s) + "'"
}

func removeLine(content, line string, remove bool) string {
	if !remove {
		return content
	}
	return strings.Replace(content, line+"\n", "", 1)
}

func TestHostileFieldDefaultsInManifestFailTheLoad(t *testing.T) {
	hostile := map[string]string{
		"colour default":    `  accent_color:` + "\n" + `    type: color` + "\n" + `    pattern: '^[0-9A-Fa-f]{6}$'` + "\n" + `    default: 'red}\directlua{x}'` + "\n",
		"asset default":     `  logo:` + "\n" + `    type: asset` + "\n" + `    pattern: '^[A-Za-z0-9_./-]+$'` + "\n",
		"permissive colour": `  accent_color:` + "\n" + `    type: color` + "\n" + `    pattern: '.*'` + "\n",
	}
	for name, def := range hostile {
		fx := newFixture(t)
		manifest := fixtureManifest
		start := strings.Index(manifest, "  accent_color:")
		end := strings.Index(manifest, "  subject:")
		manifest = manifest[:start] + def + manifest[end:]
		fx.write("templates/plain/manifest.yaml", manifest)
		reg, findings := fx.load()
		if reg != nil {
			t.Errorf("%s: accepted:\n%s", name, dump(findings))
		}
	}
}
