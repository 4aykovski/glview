package i18n

// TodoMessages are the strings of the To-Do screen, the To-Do counter in the
// header and the global T key (SPEC §3.17), filled by fillTodoEN / fillTodoRU.
type TodoMessages struct {
	Crumb     string                   // header: "To-Do · host"
	Count     func(n int) string       // header counter: "To-Do 5"; n < 0 — "To-Do 9999+"
	KeyGlobal string                   // help: T
	Action    func(name string) string // reason label; an unknown name stays as is

	ColTarget, ColTitle, ColAction, ColAgo, ColAuthor, ColProject string

	EmptyPending, EmptyDone, EmptyFiltered string

	// Values of the type filter in the filter line; its keys (state, type) are
	// filter syntax and stay as they are, like in the other filter lines.
	FilterAny, FilterMR, FilterIssue string

	KeyType, KeyDone, KeyDoneAll, KeyBrowserPage, KeyTarget string

	Done        func(ref string) string // toast: "Done: !479"
	Cleared     string                  // toast: everything done
	ConfirmAll  string                  // confirm title
	ConfirmBody func(n int) string      // n < 0 — more than 10 000
}
