package logstream

import (
	"regexp"
	"strconv"
)

var (
	// BuildKit plain progress: `#5 [2/6] RUN npm ci` (optionally staged).
	buildkitStepRe = regexp.MustCompile(`^#\d+ \[(?:[^\] ]+ )?(\d+)/(\d+)\] (.+)$`)
	// Legacy builder: `Step 2/6 : RUN npm ci`.
	legacyStepRe = regexp.MustCompile(`^Step (\d+)/(\d+) : (.+)$`)
)

// ParseBuildStep recognizes build-step lines from both the BuildKit and the
// legacy docker builder.
func ParseBuildStep(line string) (int, int, string, bool) {
	m := buildkitStepRe.FindStringSubmatch(line)
	if m == nil {
		m = legacyStepRe.FindStringSubmatch(line)
	}
	if m == nil {
		return 0, 0, "", false
	}
	step, _ := strconv.Atoi(m[1])
	total, _ := strconv.Atoi(m[2])
	return step, total, m[3], true
}
