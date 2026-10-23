package semver

import "testing"

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Version
		ok   bool
	}{
		{"v1.14.2", Version{"v", 1, 14, 2, ""}, true},
		{"1.2.3", Version{"", 1, 2, 3, ""}, true},
		{"v1.14.0-rc2", Version{"v", 1, 14, 0, "rc2"}, true},
		{"v1.0.0+build.5", Version{"v", 1, 0, 0, ""}, true},
		{"v1.0.0-rc.1+b", Version{"v", 1, 0, 0, "rc.1"}, true},
		{"latest", Version{}, false},
		{"v1.2", Version{}, false},
		{"release-2023", Version{}, false},
		{"v1.2.3.4", Version{}, false},
		{"", Version{}, false},
	} {
		got, ok := Parse(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Parse(%q) = %+v, %v", tc.in, got, ok)
		}
	}
}

func TestString(t *testing.T) {
	for _, s := range []string{"v1.14.2", "1.2.3", "v1.14.0-rc2"} {
		v, _ := Parse(s)
		if v.String() != s {
			t.Errorf("String = %q, want %q", v.String(), s)
		}
	}
}

func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.0.0-rc1", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-rc1", 1},
		{"v1.0.0-rc2", "v1.0.0-rc10", -1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-rc.1", "v1.0.0-rc.1.1", -1},
		{"v1.0.0-1", "v1.0.0-alpha", -1},
		{"v1.0.0-rc.2", "v1.0.0-rc.10", -1},
		{"v1.0.0+a", "v1.0.0+b", 0},
	} {
		a, _ := Parse(tc.a)
		b, _ := Parse(tc.b)
		if got := Compare(a, b); got != tc.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := Compare(b, a); got != -tc.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", tc.b, tc.a, got, -tc.want)
		}
	}
}

func names(s Suggestion) []string {
	var out []string
	for _, o := range s.Options {
		out = append(out, o.Name)
	}
	return out
}

func eq(a, b []string) bool {
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

func TestSuggest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []string
		base    string
		options []string
		def     string
	}{
		{"patch by default", []string{"v1.14.1", "v1.14.2", "latest"}, "v1.14.2", []string{"v1.14.3", "v1.15.0", "v2.0.0"}, "v1.14.3"},
		{"no tags", nil, "", []string{"v0.0.1", "v0.1.0", "v1.0.0"}, "v0.1.0"},
		{"only garbage", []string{"latest", "v1.2"}, "", []string{"v0.0.1", "v0.1.0", "v1.0.0"}, "v0.1.0"},
		{"no prefix", []string{"1.2.3"}, "1.2.3", []string{"1.2.4", "1.3.0", "2.0.0"}, "1.2.4"},
		{"prerelease matches minor", []string{"v1.13.4", "v1.14.0-rc2", "latest"}, "v1.13.4", []string{"v1.13.5", "v1.14.0", "v2.0.0"}, "v1.14.0"},
		{"prerelease matches patch", []string{"v1.13.4", "v1.13.5-rc1"}, "v1.13.4", []string{"v1.13.5", "v1.14.0", "v2.0.0"}, "v1.13.5"},
		{"prerelease matches major", []string{"v1.13.4", "v2.0.0-beta.1"}, "v1.13.4", []string{"v1.13.5", "v1.14.0", "v2.0.0"}, "v2.0.0"},
		{"prerelease via version", []string{"v1.14.2", "v1.16.0-rc1"}, "v1.14.2", []string{"v1.14.3", "v1.15.0", "v2.0.0", "v1.16.0"}, "v1.16.0"},
		{"old prerelease ignored", []string{"v1.14.2", "v1.14.0-rc2"}, "v1.14.2", []string{"v1.14.3", "v1.15.0", "v2.0.0"}, "v1.14.3"},
		{"only prerelease", []string{"v0.2.0-rc1"}, "", []string{"v0.0.1", "v0.1.0", "v1.0.0", "v0.2.0"}, "v0.2.0"},
		{"prerelease keeps its prefix", []string{"0.9.0-rc1"}, "", []string{"0.0.1", "0.1.0", "1.0.0", "0.9.0"}, "0.9.0"},
		{"rc2 beats rc10 order", []string{"v1.2.0-rc10", "v1.2.0-rc2", "v1.1.0"}, "v1.1.0", []string{"v1.1.1", "v1.2.0", "v2.0.0"}, "v1.2.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Suggest(tc.in)
			if s.Base != tc.base {
				t.Errorf("Base = %q, want %q", s.Base, tc.base)
			}
			if !eq(names(s), tc.options) {
				t.Errorf("Options = %v, want %v", names(s), tc.options)
			}
			if got := s.Options[s.Default].Name; got != tc.def {
				t.Errorf("Default = %q, want %q", got, tc.def)
			}
		})
	}
}

func TestSuggestMarks(t *testing.T) {
	s := Suggest([]string{"v1.13.4", "v1.14.0-rc2"})
	if !s.Options[1].Release || s.Options[0].Release {
		t.Fatalf("options = %+v", s.Options)
	}
	s = Suggest([]string{"v1.14.2", "v1.16.0-rc1"})
	if last := s.Options[len(s.Options)-1]; last.Kind != KindRelease || last.Name != "v1.16.0" {
		t.Fatalf("options = %+v", s.Options)
	}
}
