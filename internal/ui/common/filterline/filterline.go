// Package filterline is the "/" filter input of list screens: key:value
// pairs with tab completion plus free text.
package filterline

import (
	"context"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/ui/common"
)

// Key describes one filter key.
type Key struct {
	Name   string
	Desc   string   // short description shown in key suggestions
	Values []string // allowed values; nil = any value
	Multi  bool     // value is a comma separated list (labels)
	// Free accepts values outside Values: Values are only suggestions (a
	// user name or a milestone title next to any/none).
	Free bool
	// Source loads suggestions for the value being typed from the server
	// (members, labels, refs). It runs after a 300ms pause, never blocks
	// typing, and its errors are ignored. May be nil.
	Source func(ctx context.Context, prefix string) ([]Suggestion, error)
}

// Suggestion is one line of the suggestion list.
type Suggestion struct {
	Value string
	Note  string // e.g. "branch", "Anna K"
}

// ErrorKind tells what is wrong with the input.
type ErrorKind int

const (
	UnknownKey ErrorKind = iota
	BadValue
)

// ParseError is a rejected filter. The screen turns it into a translated
// hint under the input.
type ParseError struct {
	Kind    ErrorKind
	Key     string
	Value   string
	Allowed []string
}

func (e *ParseError) Error() string {
	if e.Kind == UnknownKey {
		return "unknown filter key " + e.Key
	}
	return "bad value " + e.Value + " for " + e.Key
}

// Parse splits input into key:value pairs and free text. A token with a
// colon whose key is unknown is an error, so typos are not silently
// searched as text.
func Parse(keys []Key, input string) (values map[string]string, free string, err error) {
	values = map[string]string{}
	var text []string
	for _, tok := range strings.Fields(input) {
		name, val, ok := strings.Cut(tok, ":")
		if !ok || name == "" {
			text = append(text, tok)
			continue
		}
		k := find(keys, name)
		if k == nil {
			return nil, "", &ParseError{Kind: UnknownKey, Key: name}
		}
		if val == "" || (k.Values != nil && !k.Multi && !k.Free && !contains(k.Values, val)) {
			return nil, "", &ParseError{Kind: BadValue, Key: name, Value: val, Allowed: k.Values}
		}
		if k.Multi && k.Values != nil {
			for _, v := range strings.Split(val, ",") {
				if !contains(k.Values, v) {
					return nil, "", &ParseError{Kind: BadValue, Key: name, Value: v, Allowed: k.Values}
				}
			}
		}
		values[name] = val
	}
	return values, strings.Join(text, " "), nil
}

// Format serializes values (in the order of keys) and free text back into
// input text. Empty values are skipped.
func Format(keys []Key, values map[string]string, free string) string {
	var parts []string
	for _, k := range keys {
		if v := values[k.Name]; v != "" {
			parts = append(parts, k.Name+":"+v)
		}
	}
	if free != "" {
		parts = append(parts, free)
	}
	return strings.Join(parts, " ")
}

// Complete completes the last token of input: a key name to "key:" or a
// value to "key:value ". With several candidates it extends to their common
// prefix. It returns input unchanged when nothing can be completed.
func Complete(keys []Key, input string) string {
	head, tok := splitLast(input)
	cands, prefix := candidates(keys, tok)
	if len(cands) == 0 {
		return input
	}
	if len(cands) == 1 {
		done := prefix + cands[0]
		if _, _, hasColon := strings.Cut(done, ":"); !hasColon || !strings.HasSuffix(done, ":") {
			done += " "
		}
		return head + done
	}
	common := commonPrefix(cands)
	return head + prefix + common
}

// Suggest lists the candidates for the last token of input.
func Suggest(keys []Key, input string) []string {
	_, tok := splitLast(input)
	cands, _ := candidates(keys, tok)
	return cands
}

// candidates returns the completions of tok (without the already typed
// "key:" prefix, which is returned as prefix).
func candidates(keys []Key, tok string) (cands []string, prefix string) {
	if tok == "" {
		return nil, ""
	}
	name, val, hasColon := strings.Cut(tok, ":")
	if !hasColon {
		for _, k := range keys {
			if strings.HasPrefix(k.Name, name) {
				cands = append(cands, k.Name+":")
			}
		}
		return cands, ""
	}
	k := find(keys, name)
	if k == nil || k.Values == nil {
		return nil, ""
	}
	// For lists complete the last element only.
	pre := name + ":"
	if k.Multi {
		if i := strings.LastIndex(val, ","); i >= 0 {
			pre += val[:i+1]
			val = val[i+1:]
		}
	}
	for _, v := range k.Values {
		if strings.HasPrefix(v, val) {
			cands = append(cands, v)
		}
	}
	sort.Strings(cands)
	return cands, pre
}

func splitLast(s string) (head, tok string) {
	i := strings.LastIndexAny(s, " \t")
	return s[:i+1], s[i+1:]
}

func find(keys []Key, name string) *Key {
	for i := range keys {
		if keys[i].Name == name {
			return &keys[i]
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func commonPrefix(list []string) string {
	p := list[0]
	for _, s := range list[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

const (
	debounce      = 300 * time.Millisecond
	maxSuggestion = 8
	fetchTimeout  = 15 * time.Second
)

// SuggestionsMsg carries the answer of a Key.Source. Answers whose Token is
// not the model's current one are dropped.
type SuggestionsMsg struct {
	Token  int
	Key    string
	Prefix string
	Items  []Suggestion
	Err    error
}

type debounceMsg struct{ tok int }

// Model is the filter input widget with a suggestion list.
type Model struct {
	keys    []Key
	input   textinput.Model
	err     error
	styles  *common.Styles
	current map[string]string // current filter values, shown next to key suggestions

	tok     int
	items   []Suggestion // visible suggestions
	static  []Suggestion // part of items that needs no server
	dynamic []Suggestion // answer of the last Source call
	sel     int
	width   int
}

// New creates the widget for keys.
func New(s *common.Styles, keys []Key) *Model {
	in := common.NewInput(s)
	in.Prompt = "/ "
	return &Model{keys: keys, input: in, styles: s}
}

// SetStyles applies a new theme.
func (m *Model) SetStyles(s *common.Styles) { m.styles = s; m.input.SetStyles(s.Input) }

// SetKeys replaces the keys (language change updates descriptions).
func (m *Model) SetKeys(keys []Key) { m.keys = keys }

// SetCurrent sets the values shown next to key suggestions.
func (m *Model) SetCurrent(values map[string]string) { m.current = values }

// SetWidth sets the input width.
func (m *Model) SetWidth(w int) { m.width = w; m.input.SetWidth(max(1, w-2)) }

// Open shows the input with value as the starting text.
func (m *Model) Open(value string) tea.Cmd {
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.err = nil
	cmd := m.input.Focus()
	return tea.Batch(cmd, m.refresh())
}

// Close hides the input without applying anything.
func (m *Model) Close() {
	m.input.Blur()
	m.err = nil
	m.items, m.static, m.dynamic = nil, nil, nil
	m.tok++
}

// Focused reports whether the input is open (the screen then captures keys).
func (m *Model) Focused() bool { return m.input.Focused() }

// Value returns the current text.
func (m *Model) Value() string { return m.input.Value() }

// Err returns the error of the last rejected input (nil when none).
func (m *Model) Err() error { return m.err }

// Items returns the visible suggestions (for tests and rendering).
func (m *Model) Items() []Suggestion { return m.items }

// Selected returns the index of the highlighted suggestion.
func (m *Model) Selected() int { return m.sel }

// Height is the number of lines View takes: the input plus the list.
func (m *Model) Height() int {
	n := 1 + min(len(m.items), maxSuggestion)
	if m.err != nil {
		n++
	}
	return n
}

// Result is what a key press did.
type Result int

const (
	Editing   Result = iota // keep going
	Applied                 // enter with valid input: read Parsed
	Cancelled               // esc
)

// Update handles a key press while the input is focused. On Applied the
// parsed result is available through Parsed.
func (m *Model) Update(msg tea.Msg) (Result, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return Editing, nil
	}
	switch k.String() {
	case "esc":
		m.Close()
		return Cancelled, nil
	case "down", "ctrl+n":
		if len(m.items) > 0 {
			m.sel = (m.sel + 1) % min(len(m.items), maxSuggestion)
		}
		return Editing, nil
	case "up", "ctrl+p":
		if n := min(len(m.items), maxSuggestion); n > 0 {
			m.sel = (m.sel + n - 1) % n
		}
		return Editing, nil
	case "tab":
		return Editing, m.accept()
	case "enter":
		if m.listOpen() {
			return Editing, m.accept()
		}
		if _, _, err := Parse(m.keys, m.input.Value()); err != nil {
			m.err = err
			return Editing, nil
		}
		m.input.Blur()
		m.err = nil
		m.items = nil
		m.tok++
		return Applied, nil
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	m.err = nil
	if m.input.Value() != before {
		cmd = tea.Batch(cmd, m.refresh())
	}
	return Editing, cmd
}

// HandleMsg processes the model's own messages (debounce ticks and source
// answers). The screen forwards every non-key message to it.
func (m *Model) HandleMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case debounceMsg:
		if msg.tok != m.tok || !m.Focused() {
			return nil
		}
		return m.fetch()
	case SuggestionsMsg:
		if msg.Token != m.tok || !m.Focused() {
			return nil // stale answer
		}
		if msg.Err == nil {
			m.dynamic = msg.Items
			m.rebuild()
		}
	}
	return nil
}

// DebounceMsg returns the pending debounce tick (tests deliver timers).
func (m *Model) DebounceMsg() tea.Msg { return debounceMsg{tok: m.tok} }

// word splits the text into the part before the word under the cursor and
// the word itself (the last whitespace-separated token).
func (m *Model) word() (head, tok string) { return splitLast(m.input.Value()) }

// listOpen reports whether enter would complete a suggestion rather than
// apply the filter: there is a selection that differs from what is typed.
func (m *Model) listOpen() bool {
	if len(m.items) == 0 {
		return false
	}
	_, tok := m.word()
	if tok == "" {
		return false // an empty word only lists keys: enter applies, tab picks
	}
	return m.completion(m.items[min(m.sel, len(m.items)-1)]) != tok
}

// completion is the text that replaces the current word for suggestion it.
func (m *Model) completion(it Suggestion) string {
	_, tok := m.word()
	name, val, hasColon := strings.Cut(tok, ":")
	if !hasColon {
		return it.Value // "key:"
	}
	pre := name + ":"
	if k := find(m.keys, name); k != nil && k.Multi {
		if i := strings.LastIndex(val, ","); i >= 0 {
			pre += val[:i+1]
		}
	}
	return pre + it.Value
}

func (m *Model) accept() tea.Cmd {
	if len(m.items) == 0 {
		return nil
	}
	it := m.items[min(m.sel, len(m.items)-1)]
	head, tok := m.word()
	text := m.completion(it)
	_, _, hasColon := strings.Cut(tok, ":")
	if hasColon {
		if k := find(m.keys, strings.SplitN(tok, ":", 2)[0]); k == nil || !k.Multi {
			text += " "
		}
	}
	m.input.SetValue(head + text)
	m.input.CursorEnd()
	m.err = nil
	return m.refresh()
}

// refresh recomputes the suggestions after the text changed and schedules
// the debounced server lookup for keys that have a Source.
func (m *Model) refresh() tea.Cmd {
	m.tok++
	m.dynamic = nil
	m.static = m.staticItems()
	m.rebuild()
	if k := m.sourceKey(); k != nil {
		tok := m.tok
		return tea.Tick(debounce, func(time.Time) tea.Msg { return debounceMsg{tok: tok} })
	}
	return nil
}

// sourceKey returns the key whose value is being typed, if it has a Source.
func (m *Model) sourceKey() *Key {
	_, tok := m.word()
	name, _, ok := strings.Cut(tok, ":")
	if !ok {
		return nil
	}
	if k := find(m.keys, name); k != nil && k.Source != nil {
		return k
	}
	return nil
}

func (m *Model) fetch() tea.Cmd {
	k := m.sourceKey()
	if k == nil {
		return nil
	}
	_, tok := m.word()
	_, val, _ := strings.Cut(tok, ":")
	if k.Multi {
		if i := strings.LastIndex(val, ","); i >= 0 {
			val = val[i+1:]
		}
	}
	token, name, source := m.tok, k.Name, k.Source
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := source(ctx, val)
		return SuggestionsMsg{Token: token, Key: name, Prefix: val, Items: items, Err: err}
	}
}

// staticItems lists key names or the fixed values for the current word.
func (m *Model) staticItems() []Suggestion {
	_, tok := m.word()
	name, val, hasColon := strings.Cut(tok, ":")
	if !hasColon {
		var out []Suggestion
		for _, k := range m.keys {
			if strings.HasPrefix(k.Name, name) {
				note := k.Desc
				if cur := m.current[k.Name]; cur != "" {
					note += "  " + cur
				}
				out = append(out, Suggestion{Value: k.Name + ":", Note: note})
			}
		}
		return out
	}
	k := find(m.keys, name)
	if k == nil {
		return nil
	}
	if k.Multi {
		if i := strings.LastIndex(val, ","); i >= 0 {
			val = val[i+1:]
		}
	}
	var out []Suggestion
	for _, v := range k.Values {
		out = append(out, Suggestion{Value: v})
	}
	return filterByPrefix(out, val)
}

// filterByPrefix keeps the items containing prefix, prefix matches first.
func filterByPrefix(items []Suggestion, prefix string) []Suggestion {
	p := strings.ToLower(prefix)
	var first, rest []Suggestion
	for _, it := range items {
		v := strings.ToLower(it.Value)
		switch {
		case strings.HasPrefix(v, p):
			first = append(first, it)
		case strings.Contains(v, p) || strings.Contains(strings.ToLower(it.Note), p):
			rest = append(rest, it)
		}
	}
	return append(first, rest...)
}

// rebuild merges the static and server items for display.
func (m *Model) rebuild() {
	items := append([]Suggestion(nil), m.static...)
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Value] = true
	}
	_, tok := m.word()
	_, val, _ := strings.Cut(tok, ":")
	if k := m.sourceKey(); k != nil && k.Multi {
		if i := strings.LastIndex(val, ","); i >= 0 {
			val = val[i+1:]
		}
	}
	for _, it := range filterByPrefix(m.dynamic, val) {
		if !seen[it.Value] {
			items = append(items, it)
			seen[it.Value] = true
		}
	}
	// Static prefix matches stay first; the server items follow, prefix first.
	m.items = items
	m.sel = 0
}

// Parsed parses the current text; valid after Applied.
func (m *Model) Parsed() (map[string]string, string, error) { return Parse(m.keys, m.input.Value()) }

// View renders the input line and the suggestion list below it.
func (m *Model) View() string {
	lines := []string{m.input.View()}
	valW := 0
	shown := m.items[:min(len(m.items), maxSuggestion)]
	for _, it := range shown {
		valW = max(valW, len([]rune(it.Value)))
	}
	for i, it := range shown {
		text := it.Value + strings.Repeat(" ", valW-len([]rune(it.Value)))
		if it.Note != "" {
			text += "  " + it.Note
		}
		if i == m.sel {
			lines = append(lines, m.styles.RowSelected.Render("› "+text))
		} else {
			lines = append(lines, "  "+m.styles.Muted.Render(text))
		}
	}
	return strings.Join(lines, "\n")
}
