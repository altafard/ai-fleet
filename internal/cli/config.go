package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/altafard/ai-fleet/internal/config"
	"github.com/altafard/ai-fleet/internal/gitx"
	"github.com/spf13/cobra"
)

// newConfigCmd builds `ai-fleet config get|set|list`. Wiring only: schema,
// files and merge live in internal/config.
func newConfigCmd(code *int) *cobra.Command {
	var global bool

	// localPath resolves the project-scope file; every subcommand without
	// --global involves the local scope, so an uninitialized location is a
	// usage error, not a fallback to global.
	localPath := func() (string, error) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root, err := gitx.RepoRoot(wd)
		if err != nil {
			return "", errors.New("not an ai-fleet project — run `ai-fleet init` first")
		}
		p := config.LocalPath(root)
		if _, err := os.Stat(p); err != nil {
			return "", errors.New("project is not initialized — run `ai-fleet init` first")
		}
		return p, nil
	}

	usage := func(cmd *cobra.Command, err error) error {
		cmd.SilenceUsage = true
		fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
		*code = 2
		return nil
	}

	// scopes loads what the subcommand reads: global only, or local+global.
	scopes := func() (local, globalM map[string]string, err error) {
		gp, err := config.GlobalPath()
		if err != nil {
			return nil, nil, err
		}
		if globalM, err = config.Load(gp); err != nil {
			return nil, nil, err
		}
		if global {
			return nil, globalM, nil
		}
		lp, err := localPath()
		if err != nil {
			return nil, nil, err
		}
		local, err = config.Load(lp)
		return local, globalM, err
	}

	get := &cobra.Command{
		Use:   "get <key>",
		Short: "Print a config value (local wins; --global reads the global file only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ValidateKey(args[0]); err != nil {
				return usage(cmd, err)
			}
			local, globalM, err := scopes()
			if err != nil {
				return usage(cmd, err)
			}
			cmd.SilenceUsage = true
			src := globalM
			if !global {
				if v, ok := local[args[0]]; ok {
					fmt.Fprintln(cmd.OutOrStdout(), v)
					return nil
				}
			}
			if v, ok := src[args[0]]; ok {
				fmt.Fprintln(cmd.OutOrStdout(), v)
				return nil
			}
			*code = 1 // unset: silent, scriptable — `git config` style
			return nil
		},
	}

	set := &cobra.Command{
		Use:   "set <key> [value]",
		Short: "Set a config value (omit the value to remove the key)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ValidateKey(args[0]); err != nil {
				return usage(cmd, err)
			}
			path, err := config.GlobalPath()
			if !global {
				path, err = localPath()
			}
			if err != nil {
				return usage(cmd, err)
			}
			cmd.SilenceUsage = true
			if len(args) == 1 {
				if err := config.Remove(path, args[0]); err != nil {
					return err
				}
				return nil
			}
			if err := config.ValidateValue(args[0], args[1]); err != nil {
				return usage(cmd, err)
			}
			scope, err := config.Load(path)
			if err != nil {
				return usage(cmd, err)
			}
			if err := config.CheckConflict(scope, args[0], args[1]); err != nil {
				return usage(cmd, err)
			}
			return config.Set(path, args[0], args[1])
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List config values (merged; --global lists the global file only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			local, globalM, err := scopes()
			if err != nil {
				return usage(cmd, err)
			}
			cmd.SilenceUsage = true
			merged := map[string]string{}
			for k, v := range globalM {
				merged[k] = v
			}
			for k, v := range local {
				merged[k] = v
			}
			keys := make([]string, 0, len(merged))
			for k := range merged {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", k, merged[k])
			}
			return nil
		},
	}

	c := &cobra.Command{Use: "config", Short: "Read and write persistent ai-fleet settings"}
	c.PersistentFlags().BoolVar(&global, "global", false, "operate on ~/.ai-fleet/ai-fleet.ini instead of the project")
	c.AddCommand(get, set, list)
	return c
}
