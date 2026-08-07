package config

import "github.com/altafard/ai-fleet/internal/run"

// Apply fills still-empty Options fields from merged config. Filling only
// empty fields IS the precedence rule: flags and the env fallback are
// bound before Apply runs, so flag > env > local > global falls out with
// no precedence engine anywhere.
func Apply(o *run.Options, merged map[string]string) {
	fill := func(dst *string, key string) {
		if *dst == "" {
			*dst = merged[key]
		}
	}
	fill(&o.Model, "agent.model")
	fill(&o.Effort, "agent.effort")
	fill(&o.GitProvider, "git.provider")
	fill(&o.GitRepository, "git.repository")
	fill(&o.GitAuthorName, "git.author.name")
	fill(&o.GitAuthorEmail, "git.author.email")
	fill(&o.GitToken, "git.token")
	fill(&o.GitEntityType, "git.type")
	fill(&o.GitAppID, "git.app.id")
	fill(&o.GitAppPrivateKey, "git.app.private-key")
	fill(&o.GitAppInstallationID, "git.app.installation-id")
}
