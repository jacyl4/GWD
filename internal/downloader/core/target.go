package core

// Target describes a single downloadable asset.
type Target struct {
	Name          string
	URL           string
	ExpectedHash  string
	LocalPath     string
	TempPath      string
	MinSize       int64
	Executable    bool
	SupportResume bool
}
