package github

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
)

const releasesJSON = `[
 {"tag_name":"v3.0.0-beta","prerelease":true,"assets":[{"name":"m.zip","browser_download_url":"x"}]},
 {"tag_name":"v2.9.0","draft":true,"assets":[{"name":"m.zip","browser_download_url":"x"}]},
 {"tag_name":"v2.1.0","assets":[{"name":"Mod-2.1.0.zip","content_type":"application/zip","size":4,"browser_download_url":"URL/dl/a.zip"},
   {"name":"Mod-2.1.0-alt.7z","content_type":"application/x-7z-compressed"},{"name":"src.tar.gz"},{"name":"fake.zip","content_type":"text/html"}]},
 {"tag_name":"v2.0.0","assets":[{"name":"Mod.zip"}]},
 {"tag_name":"v1.0.0","assets":[{"name":"notes.txt"}]}
]`

func fixture(t *testing.T) []Release {
	t.Helper()
	var all []Release
	if err := json.Unmarshal([]byte(releasesJSON), &all); err != nil {
		t.Fatal(err)
	}
	return all
}

func isZip(a Asset) bool { return strings.HasSuffix(a.Name, ".zip") }

func TestSelect(t *testing.T) {
	t.Parallel()
	all := fixture(t)
	r, assets, err := Select(all, "", false, nil)
	if err != nil || r.Tag != "v2.1.0" || len(assets) != 4 {
		t.Fatalf("newest, every asset kept = %q %v %v", r.Tag, assets, err)
	}
	if _, assets, err = Select(all, "", false, isZip); err != nil || len(assets) != 2 {
		t.Fatalf("keep = %v %v", assets, err)
	}
	if r, assets, err = Select(all, "2.0.0", false, isZip); err != nil || r.Tag != "v2.0.0" || len(assets) != 1 {
		t.Fatalf("requested = %q %v %v", r.Tag, assets, err)
	}
	if r, _, err = Select(all, "", true, nil); err != nil || r.Tag != "v3.0.0-beta" {
		t.Fatalf("prereleases asked for = %q %v", r.Tag, err)
	}
	if r, _, err = Select(all, "V3.0.0-beta", false, nil); err != nil || r.Tag != "v3.0.0-beta" {
		t.Fatalf("a named prerelease = %q %v", r.Tag, err)
	}
	if _, _, err = Select(all, "2.9.0", false, nil); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("a draft was picked: %v", err)
	}
	if _, _, err = Select(all, "1.0.0", false, isZip); !errors.Is(err, ErrNoAsset) {
		t.Fatalf("keep rejects all: %v", err)
	}
	if _, _, err = Select(all, "9.9.9", false, nil); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no such version: %v", err)
	}
}

// Key is stored by callers: these values were computed once and must never change.
func TestKeyGolden(t *testing.T) {
	t.Parallel()
	long := "github-" + strings.Repeat("x", 60) + "-" + strings.Repeat("y", 39) + "-e19e93751071"
	for _, c := range []struct{ owner, repo, tag, asset, want string }{
		{"o", "r", "v1", "m.zip", "github-o-r-v1-m.zip-e9c8caadd2de"},
		{"Me", "Mod", "v1.0", "mod.zip", "github-me-mod-v1.0-mod.zip-8d40e93d3dd1"},
		{"a-b", "c", "", "", "github-a-b-c-8f31f4b5cf37"},
		{"a", "b-c", "", "", "github-a-b-c-340f2b331a5c"},
		{"Pathoschild", "SMAPI", "4.1.10", "SMAPI 4.1.10 installer.zip", "github-pathoschild-smapi-4.1.10-smapi-4.1.10-installer.zip-45e176b36017"},
		{strings.Repeat("x", 60), strings.Repeat("y", 60), "v1", "é.zip", long},
		{"", "", "", "", "github--26b9ca93b9fc"},
	} {
		if got := Key(c.owner, c.repo, c.tag, c.asset); got != c.want {
			t.Errorf("Key(%q, %q, %q, %q) = %q, want %q", c.owner, c.repo, c.tag, c.asset, got, c.want)
		}
	}
}

func TestKey(t *testing.T) {
	t.Parallel()
	k := Key("Pathoschild", "StardewMods", "v2.1.0", "Mod (Final) 2.1.0.zip")
	if !strings.HasPrefix(k, "github-pathoschild-stardewmods-v2.1.0-mod-final-2.1.0.zip-") {
		t.Fatalf("key = %q", k)
	}
	if Key("a-b", "c", "t", "x.zip") == Key("a", "b-c", "t", "x.zip") {
		t.Fatal("owner/repo split folded into one key")
	}
	if Key("Me", "Mod", "t", "x.zip") != Key("me", "mod", "t", "x.zip") {
		t.Fatal("GitHub names are case-insensitive but keyed apart")
	}
	if long := Key(strings.Repeat("o", 39), strings.Repeat("r", 100), strings.Repeat("t", 100), strings.Repeat("a", 200)); len(long) > 128 {
		t.Fatalf("key is %d bytes", len(long))
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`).MatchString(Key("_a", "b_", "", "?")) {
		t.Fatal("key is not file-name-safe")
	}
}

func TestValidRepoRefusesWhatMovesTheURL(t *testing.T) {
	t.Parallel()
	for _, repo := range []string{"me/..", "./mod", "../..", "me/.", "o/r?x", "o/r#x", "o/r/s", "o /r", "", "o", "o/"} {
		if ValidRepo(repo) {
			t.Errorf("ValidRepo(%q) = true", repo)
		}
	}
	if !ValidRepo("me/mod.cfg") {
		t.Error("a dotted repo name was refused")
	}
}
