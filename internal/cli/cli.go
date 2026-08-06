// Package cli wires the cobra command tree; all behavior lives in internal/run.
package cli

import (
	"os"

	"github.com/altafard/ai-fleet/internal/run"
	"github.com/spf13/cobra"
)

// Execute parses argv and returns the process exit code.
func Execute() int {
	code := 0
	root := newRoot(&code)
	if err := root.Execute(); err != nil {
		return 2
	}
	return code
}

// newRoot builds the command tree; code receives the run's exit code.
func newRoot(code *int) *cobra.Command {
	o := run.Options{}

	unit := &cobra.Command{
		Use:   "unit",
		Short: "Run Claude Code headlessly in a container on a copy of this project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.GitToken == "" {
				o.GitToken = os.Getenv("AI_FLEET_GIT_TOKEN")
			}
			if o.Project == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				o.Project = wd
			}
			cmd.SilenceUsage = true
			*code = run.Execute(o)
			return nil
		},
	}
	f := unit.Flags()
	f.StringVarP(&o.Prompt, "prompt", "p", "", "task for Claude Code")
	f.StringVar(&o.PromptFile, "prompt-file", "", "file containing the task")
	f.StringVar(&o.Dockerfile, "dockerfile", "", "path to the environment Dockerfile (required)")
	f.StringVar(&o.Project, "project", "", "project directory (default: current directory)")
	f.StringVar(&o.Branch, "branch", "", "working branch name (default: feature/<run-id>)")
	f.StringVar(&o.GitAuthorName, "git-author-name", "", "git author name for run commits (required)")
	f.StringVar(&o.GitAuthorEmail, "git-author-email", "", "git author email for run commits (required)")
	f.StringVar(&o.GitProvider, "git-provider", "", "PR mode: git hosting provider (github)")
	f.StringVar(&o.GitRepository, "git-repository", "", "PR mode: target repository")
	f.StringVar(&o.GitToken, "git-token", "", "PR mode: auth token (prefer AI_FLEET_GIT_TOKEN)")

	deploy := &cobra.Command{Use: "deploy", Short: "Deploy agents"}
	deploy.AddCommand(unit)
	root := &cobra.Command{Use: "ai-fleet", SilenceErrors: false}
	root.Version = versionLine()
	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(deploy)
	return root
}
