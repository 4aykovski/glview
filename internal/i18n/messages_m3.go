package i18n

// M3 message groups: MR links (T34). Filled by fillM3EN / fillM3RU; the
// completeness test covers every field.

// LinksMessages are the strings of the MR link overlay (L) and the links
// config rules.
type LinksMessages struct {
	Key        string                  // footer label of L
	Title      func(ref string) string // overlay title: "Links · !479"
	Hints      string                  // overlay footer hints
	NoLinks    string                  // toast: the MR has no links
	What       string                  // "copied: link"
	BadRule    func(detail string) string
	LoadFailed string // toast: the MR details for the links failed
}
