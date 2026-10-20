// Package i18n holds the typed UI strings for English and Russian. Only
// chrome is translated (tabs, hints, toasts, errors, relative time); GitLab
// data and pipeline statuses are never translated.
package i18n

import (
	"fmt"
	"time"
)

type Lang string

const (
	EN Lang = "en"
	RU Lang = "ru"
)

// Parse maps a config value to a language; anything unknown is English.
func Parse(s string) Lang {
	if s == string(RU) {
		return RU
	}
	return EN
}

// Next returns the other language (ctrl+l toggles en↔ru).
func (l Lang) Next() Lang {
	if l == RU {
		return EN
	}
	return RU
}

// Get returns the messages for l.
func Get(l Lang) *Messages {
	if l == RU {
		return &ru
	}
	return &en
}

// PluralForms are the word forms for one/few/many. English uses One and Many.
type PluralForms struct{ One, Few, Many string }

// Plural picks the form for n according to the language's rule.
func Plural(l Lang, n int, f PluralForms) string {
	if n < 0 {
		n = -n
	}
	if l == RU {
		switch {
		case n%10 == 1 && n%100 != 11:
			return f.One
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return f.Few
		default:
			return f.Many
		}
	}
	if n == 1 {
		return f.One
	}
	return f.Many
}

// Count formats "<n> <form>", e.g. "21 тег", "5 projects".
func Count(l Lang, n int, f PluralForms) string {
	return fmt.Sprintf("%d %s", n, Plural(l, n, f))
}

type unit struct {
	d          time.Duration
	shortEN    string
	shortRU    string
	longEN     PluralForms
	longRU     PluralForms
	roundUnder time.Duration // use this unit while d < roundUnder
}

var units = []unit{
	{time.Minute, "m", "м", PluralForms{"minute", "", "minutes"}, PluralForms{"минуту", "минуты", "минут"}, time.Hour},
	{time.Hour, "h", "ч", PluralForms{"hour", "", "hours"}, PluralForms{"час", "часа", "часов"}, 24 * time.Hour},
	{24 * time.Hour, "d", "д", PluralForms{"day", "", "days"}, PluralForms{"день", "дня", "дней"}, 7 * 24 * time.Hour},
	{7 * 24 * time.Hour, "w", "н", PluralForms{"week", "", "weeks"}, PluralForms{"неделю", "недели", "недель"}, 30 * 24 * time.Hour},
	{30 * 24 * time.Hour, "mo", "мес", PluralForms{"month", "", "months"}, PluralForms{"месяц", "месяца", "месяцев"}, 365 * 24 * time.Hour},
	{365 * 24 * time.Hour, "y", "г", PluralForms{"year", "", "years"}, PluralForms{"год", "года", "лет"}, 0},
}

func pickUnit(d time.Duration) (unit, int) {
	for _, u := range units {
		if u.roundUnder == 0 || d < u.roundUnder {
			return u, int(d / u.d)
		}
	}
	u := units[len(units)-1]
	return u, int(d / u.d)
}

// RelTimeShort formats an age compactly: en "4m 1h 2d 3w 1mo 1y", ru "4м 1ч 2д 3н 1мес 1г".
func RelTimeShort(l Lang, d time.Duration) string {
	if d < time.Minute {
		if l == RU {
			return "<1м"
		}
		return "<1m"
	}
	u, n := pickUnit(d)
	if l == RU {
		return fmt.Sprintf("%d%s", n, u.shortRU)
	}
	return fmt.Sprintf("%d%s", n, u.shortEN)
}

// RelTimeLong formats an age as a phrase: "4 minutes ago" / "4 минуты назад".
func RelTimeLong(l Lang, d time.Duration) string {
	if d < time.Minute {
		if l == RU {
			return "только что"
		}
		return "just now"
	}
	u, n := pickUnit(d)
	if l == RU {
		return fmt.Sprintf("%s назад", Count(RU, n, u.longRU))
	}
	return fmt.Sprintf("%s ago", Count(EN, n, u.longEN))
}
