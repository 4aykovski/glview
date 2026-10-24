// Package semver parses tag names that look like semantic versions and
// suggests the name of the next tag (SPEC §3.10, §11.2).
package semver

import (
	"regexp"
	"strconv"
	"strings"
)

// Version is a parsed tag name. Build metadata is dropped: it takes no part
// in precedence.
type Version struct {
	Prefix              string // "v" or ""
	Major, Minor, Patch int
	Pre                 string // "rc2" in v1.14.0-rc2, "" for a release
}

var re = regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

// Parse reads [v]MAJOR.MINOR.PATCH[-pre][+build]; ok is false for any
// other name.
func Parse(s string) (Version, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	v := Version{Prefix: m[1], Pre: m[5]}
	for i, dst := range []*int{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.Atoi(m[i+2])
		if err != nil {
			return Version{}, false
		}
		*dst = n
	}
	return v, true
}

// String is the tag name: prefix, core and prerelease.
func (v Version) String() string {
	s := v.Prefix + strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare orders versions by SemVer 2.0 precedence (a prerelease is lower
// than its release); it returns -1, 0 or 1. Digit runs inside a prerelease
// identifier compare as numbers, so rc2 < rc10.
func Compare(a, b Version) int {
	if c := cmpInt(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpInt(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpInt(a.Patch, b.Patch); c != 0 {
		return c
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	as, bs := strings.Split(a.Pre, "."), strings.Split(b.Pre, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := cmpIdent(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// cmpIdent compares prerelease identifiers: numeric ones are lower than
// alphanumeric ones; alphanumeric ones compare chunk by chunk (text
// lexically, digit runs numerically).
func cmpIdent(a, b string) int {
	da, db := isDigits(a), isDigits(b)
	switch {
	case da && db:
		return cmpDigits(a, b)
	case da:
		return -1
	case db:
		return 1
	}
	for a != "" && b != "" {
		ca, cb := chunk(&a), chunk(&b)
		var c int
		if isDigits(ca) && isDigits(cb) {
			c = cmpDigits(ca, cb)
		} else {
			c = strings.Compare(ca, cb)
		}
		if c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

// chunk cuts the leading run of digits or non-digits off s.
func chunk(s *string) string {
	digit := (*s)[0] >= '0' && (*s)[0] <= '9'
	i := 1
	for i < len(*s) && ((*s)[i] >= '0' && (*s)[i] <= '9') == digit {
		i++
	}
	out := (*s)[:i]
	*s = (*s)[i:]
	return out
}

// cmpDigits compares non-negative decimal strings of any length.
func cmpDigits(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if c := cmpInt(len(a), len(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// Kind names a suggested bump.
type Kind string

const (
	KindPatch   Kind = "patch"
	KindMinor   Kind = "minor"
	KindMajor   Kind = "major"
	KindRelease Kind = "release"
)

// Option is one suggested tag name.
type Option struct {
	Kind Kind
	Name string
	// Release marks a patch/minor/major name that is also the release of
	// the newest prerelease tag (shown as "minor · release").
	Release bool
}

// Suggestion is the set of proposed names for a new tag.
type Suggestion struct {
	Base    string   // the base tag, "" when there is no release semver tag
	Options []Option // patch, minor, major, then a separate release if none matched
	Default int      // index of the option chosen by default
}

// Suggest proposes names for the next tag given the existing tag names.
// Names that are not semantic versions are skipped.
func Suggest(names []string) Suggestion {
	var base, top, topPre *Version
	for _, n := range names {
		v, ok := Parse(n)
		if !ok {
			continue
		}
		if top == nil || Compare(v, *top) > 0 {
			top = &v
		}
		if v.Pre == "" {
			if base == nil || Compare(v, *base) > 0 {
				base = &v
			}
		} else if topPre == nil || Compare(v, *topPre) > 0 {
			topPre = &v
		}
	}

	var s Suggestion
	var b Version
	switch {
	case base != nil:
		b = *base
		s.Base = b.String()
	case topPre != nil:
		b = Version{Prefix: topPre.Prefix}
	default:
		b = Version{Prefix: "v"}
	}
	opts := []Option{
		{Kind: KindPatch, Name: Version{b.Prefix, b.Major, b.Minor, b.Patch + 1, ""}.String()},
		{Kind: KindMinor, Name: Version{b.Prefix, b.Major, b.Minor + 1, 0, ""}.String()},
		{Kind: KindMajor, Name: Version{b.Prefix, b.Major + 1, 0, 0, ""}.String()},
	}
	s.Default = 0
	if base == nil {
		s.Default = 1
	}
	// The newest tag is a prerelease of a version above the base: its
	// release is what is usually tagged next.
	if top != nil && top.Pre != "" {
		rel := Version{top.Prefix, top.Major, top.Minor, top.Patch, ""}
		core := Version{b.Prefix, rel.Major, rel.Minor, rel.Patch, ""}
		matched := -1
		for i, o := range opts {
			if o.Name == core.String() {
				matched = i
			}
		}
		if matched >= 0 {
			opts[matched].Release = true
			s.Default = matched
		} else {
			opts = append(opts, Option{Kind: KindRelease, Name: rel.String()})
			s.Default = len(opts) - 1
		}
	}
	s.Options = opts
	return s
}
