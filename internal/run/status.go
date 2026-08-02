package run

import (
	"encoding/json"
	"os"
)

// Status is status.json — written at collect, rewritten after publish.
type Status struct {
	RunID       string `json:"run_id"`
	BaselineRef string `json:"baseline_ref"`
	BaselineSHA string `json:"baseline_sha"`
	Branch      string `json:"branch"`
	ImageID     string `json:"image_id"`
	ExitCode    int    `json:"exit_code"`
	CommitCount int    `json:"commit_count"`
	PRURL       string `json:"pr_url,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
}

// WriteStatus writes s to path as indented JSON with a trailing newline,
// replacing any existing file.
func WriteStatus(path string, s Status) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
