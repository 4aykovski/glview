package i18n

// T35 message group: the MR Diff tab and line comments. Filled by fillT35EN /
// fillT35RU; the completeness test covers every field.

// DiffMessages are the strings of the Diff tab of the MR screen (SPEC §3.5)
// and of the line comment form (SPEC §3.16).
type DiffMessages struct {
	Tab string // tab name

	// File header badges
	New, Deleted, Generated, TooLarge string

	Hidden       string // body of a file whose diff GitLab hid
	NoText       string // body of a file without a text diff
	Empty        string // the MR has no changes
	Loading      string
	CommentTitle func(ref, where string) string // "Comment · !479 · path:42"
	StaleHint    string                         // toast hint when the line is gone

	// Footer labels
	KeyLine, KeyFile, KeyShift, KeyToLine, KeyThread, KeyDiff, KeyBrowser string
}
