package i18n

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestCompleteness fails when a string is empty or a func is nil in any language.
func TestCompleteness(t *testing.T) {
	for _, l := range []Lang{EN, RU} {
		var missing []string
		walk(reflect.ValueOf(*Get(l)), string(l), &missing)
		if len(missing) > 0 {
			t.Errorf("%s: missing translations:\n  %s", l, strings.Join(missing, "\n  "))
		}
	}
}

func walk(v reflect.Value, path string, missing *[]string) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			walk(v.Field(i), path+"."+v.Type().Field(i).Name, missing)
		}
	case reflect.String:
		if v.String() == "" {
			*missing = append(*missing, path)
		}
	case reflect.Func:
		if v.IsNil() {
			*missing = append(*missing, path)
		}
	}
}

func TestPluralRU(t *testing.T) {
	f := PluralForms{"тег", "тега", "тегов"}
	cases := map[int]string{
		0: "тегов", 1: "тег", 2: "тега", 4: "тега", 5: "тегов", 11: "тегов", 12: "тегов",
		14: "тегов", 21: "тег", 22: "тега", 25: "тегов", 101: "тег", 111: "тегов", 112: "тегов",
	}
	for n, want := range cases {
		if got := Plural(RU, n, f); got != want {
			t.Errorf("Plural(ru, %d) = %q, want %q", n, got, want)
		}
	}
}

func TestPluralEN(t *testing.T) {
	f := PluralForms{One: "tag", Many: "tags"}
	if Plural(EN, 1, f) != "tag" || Plural(EN, 0, f) != "tags" || Plural(EN, 21, f) != "tags" {
		t.Fatal("english plural")
	}
}

func TestSpecExamples(t *testing.T) {
	ru := Get(RU)
	for n, want := range map[int]string{1: "1 тег", 3: "3 тега", 11: "11 тегов", 21: "21 тег", 58: "58 тегов"} {
		if got := ru.Plural.Tags(n); got != want {
			t.Errorf("Tags(%d) = %q, want %q", n, got, want)
		}
	}
	if got := ru.Plural.GitLabProjects(5); got != "5 GitLab-проектов" {
		t.Errorf("got %q", got)
	}
	if got := Get(EN).Plural.GitLabProjects(1); got != "1 GitLab project" {
		t.Errorf("got %q", got)
	}
}

func TestRelTime(t *testing.T) {
	cases := []struct {
		d                time.Duration
		shortEN, shortRU string
		longRU           string
	}{
		{30 * time.Second, "<1m", "<1м", "только что"},
		{4 * time.Minute, "4m", "4м", "4 минуты назад"},
		{90 * time.Minute, "1h", "1ч", "1 час назад"},
		{5 * time.Hour, "5h", "5ч", "5 часов назад"},
		{2 * 24 * time.Hour, "2d", "2д", "2 дня назад"},
		{21 * 24 * time.Hour, "3w", "3н", "3 недели назад"},
		{45 * 24 * time.Hour, "1mo", "1мес", "1 месяц назад"},
		{800 * 24 * time.Hour, "2y", "2г", "2 года назад"},
		{21 * time.Minute, "21m", "21м", "21 минуту назад"},
	}
	for _, c := range cases {
		if got := RelTimeShort(EN, c.d); got != c.shortEN {
			t.Errorf("short en %v = %q, want %q", c.d, got, c.shortEN)
		}
		if got := RelTimeShort(RU, c.d); got != c.shortRU {
			t.Errorf("short ru %v = %q, want %q", c.d, got, c.shortRU)
		}
		if got := RelTimeLong(RU, c.d); got != c.longRU {
			t.Errorf("long ru %v = %q, want %q", c.d, got, c.longRU)
		}
	}
	if got := RelTimeLong(EN, time.Hour); got != "1 hour ago" {
		t.Errorf("long en = %q", got)
	}
}

func TestParseAndNext(t *testing.T) {
	if Parse("ru") != RU || Parse("") != EN || Parse("de") != EN {
		t.Fatal("parse")
	}
	if EN.Next() != RU || RU.Next() != EN {
		t.Fatal("next")
	}
}

func TestChipApprovals(t *testing.T) {
	cases := []struct {
		have, req int
		en, ru    string
	}{
		{1, 2, "1/2 approved", "1/2 апрувов"},
		{3, 2, "3 approved", "апрувов: 3"},
		{2, 2, "2 approved", "апрувов: 2"},
		{2, 0, "2 approved", "апрувов: 2"},
	}
	for _, c := range cases {
		if got := Get(EN).MR.ChipApprovals(c.have, c.req); got != c.en {
			t.Errorf("en %d/%d = %q, want %q", c.have, c.req, got, c.en)
		}
		if got := Get(RU).MR.ChipApprovals(c.have, c.req); got != c.ru {
			t.Errorf("ru %d/%d = %q, want %q", c.have, c.req, got, c.ru)
		}
	}
}
