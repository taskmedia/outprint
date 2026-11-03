package docslink

import "testing"

func TestVersionPath(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"release", "0.3.2", "0.3"},
		{"release first patch", "0.3.0", "0.3"},
		{"release with v prefix", "v1.12.4", "1.12"},
		{"release with surrounding space", " 0.3.2\n", "0.3"},
		{"multi digit parts", "10.20.30", "10.20"},

		{"dev", "dev", "dev"},
		{"dev suffix", "0.3.2-dev", "dev"},
		{"dev suffix with v", "v0.3.2-dev", "dev"},
		{"dirty", "0.3.2-dirty", "dev"},
		{"git describe after a tag", "v0.3.2-5-g1a2b3c4", "dev"},
		{"git describe after a tag, dirty", "v0.3.2-5-g1a2b3c4-dirty", "dev"},
		{"commit only", "1a2b3c4", "dev"},
		{"long commit only", "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", "dev"},
		{"commit only, dirty", "1a2b3c4-dirty", "dev"},
		{"release candidate that is dirty", "0.4.0-rc.1-dirty", "dev"},

		{"pre-release has no docs of its own", "0.4.0-rc.1", ""},
		{"pre-release alpha", "1.0.0-alpha", ""},
		{"empty", "", ""},
		{"unparsable", "banana", ""},
		{"two parts only", "0.3", ""},
		{"four parts", "0.3.2.1", ""},
		{"leading zero", "01.2.3", ""},
		{"build metadata", "0.3.2+abc", ""},
		{"path traversal", "../../x", ""},
		{"url like", "0.3.2/evil", ""},
		{"unicode digits", "٠.٣.٢", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VersionPath(tt.version); got != tt.want {
				t.Fatalf("VersionPath(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestHome(t *testing.T) {
	tests := []struct{ version, want string }{
		{"0.3.2", "https://outprint.task.media/0.3/"},
		{"dev", "https://outprint.task.media/dev/"},
		{"", "https://outprint.task.media/"},
		{"banana", "https://outprint.task.media/"},
	}
	for _, tt := range tests {
		if got := Home(tt.version); got != tt.want {
			t.Errorf("Home(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestErrorURL(t *testing.T) {
	const page = "/reference/error-messages/"
	tests := []struct{ name, version, code, want string }{
		{"release", "0.3.2", "no_binding", "https://outprint.task.media/0.3" + page + "#no_binding"},
		{"v prefix", "v0.3.2", "enum", "https://outprint.task.media/0.3" + page + "#enum"},
		{"dev", "dev", "required", "https://outprint.task.media/dev" + page + "#required"},
		{"unknown version goes to the latest docs", "", "required", "https://outprint.task.media" + page + "#required"},
		{"code with digits", "0.3.2", "binding_v2", "https://outprint.task.media/0.3" + page + "#binding_v2"},
		{"empty code has no link", "0.3.2", "", ""},
		{"code with a slash has no link", "0.3.2", "a/b", ""},
		{"code with a quote has no link", "0.3.2", `a"b`, ""},
		{"code with a space has no link", "0.3.2", "a b", ""},
		{"upper case has no link", "0.3.2", "Enum", ""},
		{"too long has no link", "0.3.2", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorURL(tt.version, tt.code); got != tt.want {
				t.Fatalf("ErrorURL(%q, %q) = %q, want %q", tt.version, tt.code, got, tt.want)
			}
		})
	}
}
