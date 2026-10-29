package github

import (
	"slices"
	"testing"
)

func names(assets []Asset) []string {
	out := make([]string, len(assets))
	for i, a := range assets {
		out[i] = a.Name
	}
	return out
}

func TestInstallable(t *testing.T) {
	t.Parallel()
	all := []Asset{
		{Name: "Mod-1.2.zip"},
		{Name: "Mod-1.2-source.zip"},
		{Name: "src.zip"},
		{Name: "checksums.zip"},
		{Name: "Mod-1.2-Linux.zip"},
		{Name: "Mod-1.2-win64.zip"},
		{Name: "Mod-1.2-macOS.zip"},
	}
	if got := names(Installable(all, "linux")); !slices.Equal(got, []string{"Mod-1.2.zip", "Mod-1.2-Linux.zip"}) {
		t.Errorf("linux kept %v", got)
	}
	if got := names(Installable(all, "windows")); !slices.Equal(got, []string{"Mod-1.2.zip", "Mod-1.2-win64.zip"}) {
		t.Errorf("windows kept %v", got)
	}
	// Only another platform's build ships, so it stays: the author meant it for everyone.
	if got := names(Installable([]Asset{{Name: "Mod-Windows.zip"}, {Name: "Mod-src.zip"}}, "linux")); !slices.Equal(got, []string{"Mod-Windows.zip"}) {
		t.Errorf("kept %v", got)
	}
}

// Shape is stored by callers to remember a choice: these values must never change.
func TestShape(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"MyMod-1.2.0-linux.zip":        "mymod linux",
		"MyMod v1.3 linux.ZIP":         "mymod linux",
		"SMAPI 4.1.10 installer.zip":   "smapi installer",
		"Mod_1_2_3-win64.7z":           "mod win",
		"MelonLoader.Installer.exe":    "melonloader installer",
		"app-v2-x86_64.AppImage":       "app x",
		"":                             "",
		"1.2.3":                        "",
		"checksums.sha256":             "checksums",
		"name.with.dots-1.0.tar.gz":    "name with dots tar",
		"Ünï-1.0.zip":                  "n",
		"mod-v10-v11.zip":              "mod",
		"mod-2024-01-02-nightly.zip":   "mod nightly",
		"mod-1.0-rc1.zip":              "mod rc",
		"UPPER.lower.Mixed-3.14.1.zip": "upper lower mixed",
	} {
		if got := Shape(in); got != want {
			t.Errorf("Shape(%q) = %q, want %q", in, got, want)
		}
	}
}
