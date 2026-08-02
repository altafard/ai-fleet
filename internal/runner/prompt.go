// Package runner owns everything that crosses into the container: the
// prompt template, the default git style guidelines and the entrypoint.
package runner

import (
	_ "embed"
	"strings"
	"text/template"
)

//go:embed prompt.md.tmpl
var promptTmpl string

//go:embed guidelines.md
var defaultGuidelines string

var tmpl = template.Must(template.New("prompt").Parse(promptTmpl))

// RenderPrompt wraps the user's task in the deterministic prompt contract.
// GitStyleGuidelines is always the embedded default in v1; the field is the
// extension point for custom guideline discovery later.
func RenderPrompt(branch, task string) (string, error) {
	var b strings.Builder
	err := tmpl.Execute(&b, struct {
		Branch, Task, GitStyleGuidelines string
	}{branch, strings.TrimSpace(task), strings.TrimSpace(defaultGuidelines)})
	return b.String(), err
}
