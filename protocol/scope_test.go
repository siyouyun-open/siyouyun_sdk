package protocol

import "testing"

// The scope value syntax is a "rule shared by three parties" (local application filtering /
// daemon fan-out filtering / manifest validation), so the behavior tests here are the
// executable specification of that rule.

func TestParseScopeValueValid(t *testing.T) {
	cases := []struct {
		raw   string
		want  ScopeValue
		whole bool
	}{
		{"space", ScopeValue{Space: ScopeSpaceOwn}, true},
		{"space.all", ScopeValue{Space: ScopeSpaceOwn}, true},
		{"space.music", ScopeValue{Space: ScopeSpaceOwn, Key: "music"}, false},
		{"space.music/2024", ScopeValue{Space: ScopeSpaceOwn, Key: "music", Sub: "/2024"}, false},
		{"space.MUSIC/2024/raw", ScopeValue{Space: ScopeSpaceOwn, Key: "music", Sub: "/2024/raw"}, false},
		{"space.music/", ScopeValue{Space: ScopeSpaceOwn, Key: "music"}, false},
		{"app", ScopeValue{Space: ScopeSpaceApp}, true},
		{"all.photos", ScopeValue{Space: ScopeSpaceAll, Key: "photos"}, false},
		{"  all.movies/2024  ", ScopeValue{Space: ScopeSpaceAll, Key: "movies", Sub: "/2024"}, false},
	}
	for _, tc := range cases {
		got, err := ParseScopeValue(tc.raw)
		if err != nil {
			t.Fatalf("ParseScopeValue(%q): %v", tc.raw, err)
		}
		if got != tc.want {
			t.Errorf("ParseScopeValue(%q) = %+v, want %+v", tc.raw, got, tc.want)
		}
		if got.WholeSpace() != tc.whole {
			t.Errorf("ParseScopeValue(%q).WholeSpace() = %v", tc.raw, got.WholeSpace())
		}
		if round := got.String(); round != tc.want.String() {
			t.Errorf("ParseScopeValue(%q).String() = %q, want %q", tc.raw, round, tc.want.String())
		}
	}
}

func TestParseScopeValueInvalid(t *testing.T) {
	for _, raw := range []string{
		"", "   ",
		"device",            // the old token name is no longer a space axis
		"user",              // same as above
		"space.unknownkey",  // unregistered key
		"space.music//2024", // empty segments must not be silently folded
		"space.music/../etc",
		"app.photos", // app does not accept path narrowing
		"all.music/a//b",
	} {
		if _, err := ParseScopeValue(raw); err == nil {
			t.Errorf("ParseScopeValue(%q) should report an error", raw)
		}
	}
}

func TestScopeValueContains(t *testing.T) {
	must := func(raw string) ScopeValue {
		v, err := ParseScopeValue(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		return v
	}
	cases := []struct {
		outer, inner string
		want         bool
	}{
		{"space", "space", true},
		{"space", "space.music", true},
		{"space", "space.music/2024", true},
		{"space.music", "space.music/2024", true},
		{"space.music", "space.music", true},
		{"space.music/2024", "space.music", false}, // a subpath cannot cover its parent
		{"space.music", "space.movies", false},     // different keys are incomparable
		{"space.music/2024", "space.music/2024/08", true},
		{"space.music/2024", "space.music/20240", false}, // segment boundary
		{"all", "space", true},                           // device level covers space level
		{"all.music", "space.music", true},
		{"all", "all.movies", true},
		{"space", "all", false}, // the reverse does not hold
		{"space.music", "all.music", false},
		{"app", "space", false}, // app is incomparable with the space axis
		{"space", "app", false},
		{"app", "app", true},
	}
	for _, tc := range cases {
		if got := must(tc.outer).Contains(must(tc.inner)); got != tc.want {
			t.Errorf("%q.Contains(%q) = %v, want %v", tc.outer, tc.inner, got, tc.want)
		}
	}
	// The zero value covers nothing and is covered by nothing.
	if (ScopeValue{}).Contains(must("space")) || must("space").Contains(ScopeValue{}) {
		t.Error("the zero value must not contain anything")
	}
}

func TestScopeKeyPath(t *testing.T) {
	cases := []struct {
		raw  string
		path string
		ok   bool
	}{
		{"space", "/", true},
		{"all", "/", true},
		{"app", "", false}, // app is a private directory and is not an in-pool path
		{"space.music", "/Music", true},
		{"space.music/2024", "/Music/2024", true},
		{"space.music/2024/raw", "/Music/2024/raw", true},
		{"all.photos", "/Photos", true},
	}
	for _, tc := range cases {
		value, err := ParseScopeValue(tc.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.raw, err)
		}
		got, ok := value.ScopeKeyPath()
		if got != tc.path || ok != tc.ok {
			t.Errorf("%q.ScopeKeyPath() = (%q,%v), want (%q,%v)", tc.raw, got, ok, tc.path, tc.ok)
		}
	}
}

func TestUFIPoolRel(t *testing.T) {
	cases := []struct {
		ufi string
		rel string
		ok  bool
	}{
		{"/ufi/pool-raw/syspool/Music/2024/a.mp3", "/Music/2024/a.mp3", true},
		{"/ufi/pool-raw/alice_private_syspool/Music/a.mp3", "/Music/a.mp3", true},
		{"/ufi/pool-raw/syspool/", "/", true},
		{"/ufi/pool-raw/syspool", "/", true},
		{"/other/format/Music/a.mp3", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		rel, ok := UFIPoolRel(tc.ufi)
		if rel != tc.rel || ok != tc.ok {
			t.Errorf("UFIPoolRel(%q) = (%q,%v), want (%q,%v)", tc.ufi, rel, ok, tc.rel, tc.ok)
		}
	}
}

func TestPathWithin(t *testing.T) {
	cases := []struct {
		want, rel string
		ok        bool
	}{
		{"/", "/Music/a.mp3", true},
		{"/Music", "/Music/a.mp3", true},
		{"/Music", "/Music", true},
		{"/Music", "/Music/2024/a.mp3", true},
		{"/Music", "/Music2024/a.mp3", false}, // segment boundary: a bare prefix does not count
		{"/Music/2024", "/Music/a.mp3", false},
		{"/Music", "/Documents/a.txt", false},
	}
	for _, tc := range cases {
		if got := PathWithin(tc.want, tc.rel); got != tc.ok {
			t.Errorf("PathWithin(%q,%q) = %v, want %v", tc.want, tc.rel, got, tc.ok)
		}
	}
}

func TestScopeKeyForDir(t *testing.T) {
	cases := []struct {
		dir string
		key string
		ok  bool
	}{
		{"/Music", "music", true},
		{"/Music/2024", "music/2024", true},
		{"/Music/2024/raw", "music/2024/raw", true},
		{"/Music/", "music", true},
		{"/Photos", "photos", true},
		{"/Documents", "documents", true},
		{"/Music2024", "", false}, // segment boundary: not a subdirectory of /Music
		{"/Unknown", "", false},
		{"/", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := ScopeKeyForDir(tc.dir)
		if got != tc.key || ok != tc.ok {
			t.Errorf("ScopeKeyForDir(%q) = (%q,%v), want (%q,%v)", tc.dir, got, ok, tc.key, tc.ok)
		}
	}
}

func TestScopeKeyNames(t *testing.T) {
	names := ScopeKeyNames()
	if len(names) == 0 {
		t.Fatal("the key table must not be empty")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("key names must be sorted stably: %v", names)
		}
	}
	if _, ok := ScopeKeyDir("MUSIC"); !ok {
		t.Error("key lookup should be case-insensitive")
	}
	if _, ok := ScopeKeyDir("nope"); ok {
		t.Error("an unregistered key must not resolve")
	}
}
