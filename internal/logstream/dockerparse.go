package logstream

import (
	"regexp"
	"strconv"
)

var buildStepRe = regexp.MustCompile(`^#\d+ \[(?:[^\] ]+ )?(\d+)/(\d+)\] (.+)$`)

// ParseBuildStep recognizes step lines of `docker build --progress=plain`.
func ParseBuildStep(line string) (int, int, string, bool) {
	m := buildStepRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, "", false
	}
	step, _ := strconv.Atoi(m[1])
	total, _ := strconv.Atoi(m[2])
	return step, total, m[3], true
}
