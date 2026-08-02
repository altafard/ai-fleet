package run

import "fmt"

// MergeInstructions is printed verbatim after a successful bundle-only run.
func MergeInstructions(runRel, branch, baseBranch string, commits int) string {
	bundle := runRel + "/out/run.bundle"
	return fmt.Sprintf(
		"Run complete: branch %s (%d commits)\n"+
			"Fetch:  git fetch %s %s:%s\n"+
			"Review: git log -p %s..%s\n"+
			"Merge:  git merge %s\n",
		branch, commits, bundle, branch, branch, baseBranch, branch, branch)
}
