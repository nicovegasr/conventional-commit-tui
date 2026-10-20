// Package history reads immutable Git history without depending on the UI.
package history

// Change describes a path in a non-merge commit. Binary line counts are unknown.
type Change struct {
	Path         string `json:"path"`
	PreviousPath string `json:"previous_path,omitempty"`
	Kind         string `json:"kind"`
	Added        int64  `json:"added"`
	Deleted      int64  `json:"deleted"`
	Binary       bool   `json:"binary,omitempty"`
}

// Commit is one immutable Git commit and its non-merge changes.
type Commit struct {
	OID       string   `json:"oid"`
	Parents   []string `json:"parents"`
	Author    string   `json:"author"`
	Email     string   `json:"email"`
	Timestamp int64    `json:"timestamp"`
	Message   string   `json:"message"`
	Changes   []Change `json:"changes,omitempty"`
}
