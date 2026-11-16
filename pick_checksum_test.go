package github

import (
	"slices"
	"testing"
)

func TestInstallableHashNamedArchive(t *testing.T) {
	for _, name := range []string{
		"app-1.0-linux-amd64-sha256.zip",
		"tool-sha256-linux.zip",
		"app-1.0-sha512.deb",
	} {
		got := Installable([]Asset{{Name: name}, {Name: "README.md"}}, "linux")
		var names []string
		for _, a := range got {
			names = append(names, a.Name)
		}
		if !slices.Contains(names, name) {
			t.Fatalf("%s dropped; kept %v", name, names)
		}
	}
}
