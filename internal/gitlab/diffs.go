package gitlab

// DiffRefs are the three SHAs a diff position is anchored to.
type DiffRefs struct{ BaseSHA, StartSHA, HeadSHA string }

// DiffFile is one file of an MR diff (REST /merge_requests/:iid/diffs).
type DiffFile struct {
	OldPath, NewPath                  string
	NewFile, RenamedFile, DeletedFile bool
	Generated                         bool   // generated_file; false when the field is missing
	TooLarge                          bool   // too_large or collapsed: the server hid the diff
	Diff                              string // unified diff hunks as the API returns them (with @@, no ---/+++)
}

// DiffPosition is the position of a line comment
// (POST /merge_requests/:iid/discussions).
type DiffPosition struct {
	DiffRefs
	OldPath, NewPath string
	// OldLine/NewLine: 0 means none — an added line has only NewLine, a
	// removed one only OldLine, a context line both.
	OldLine, NewLine int
}
