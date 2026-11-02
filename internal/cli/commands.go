package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/config"
	"github.com/ardacoskun/codehop/internal/paths"
	"github.com/ardacoskun/codehop/internal/preview"
	"github.com/ardacoskun/codehop/internal/shell"
)

func newPreviewCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:    "preview <path>",
		Short:  "Print the picker preview for a project directory",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			preview.Write(cmd.Context(), a.stdout, args[0], a.now())
			return nil
		},
	}
}

const defaultCdCmd = "cdp"

func newInitCmd(a *app) *cobra.Command {
	var (
		cmdName string
		cdCmd   string
		noCmd   bool
	)
	cmd := &cobra.Command{
		Use:   "init <shell>",
		Short: "Print the shell integration (zsh, bash, fish, powershell)",
		Long: `Print the shell integration. It defines:

  <cmd> [project]    open a project; paths and flags go to the editor unchanged
                     (default name: the editor's command, e.g. "code")
  cdp [project]      cd into a project
  <action> [project] one command per [actions] entry in the config

All of them complete project names with Tab.

  zsh         eval "$(codehop init zsh)"            in ~/.zshrc, after compinit
  bash        eval "$(codehop init bash)"           in ~/.bashrc
  fish        codehop init fish | source            in ~/.config/fish/config.fish
  powershell  Invoke-Expression (& { (codehop init powershell | Out-String) })
                                                    in $PROFILE`,
		Example: `  eval "$(codehop init zsh)"                  # code + cdp
  eval "$(codehop init zsh --cmd cursor)"     # wrap cursor instead
  eval "$(codehop init zsh --no-cmd)"         # leave the editor command alone`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: shell.Supported,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("cmd") {
				ed, err := a.editor()
				if err != nil {
					return err
				}
				cmdName = ed.CommandName()
			}
			if noCmd {
				cmdName = ""
			}
			script, err := shell.Render(args[0], shell.Options{Cmd: cmdName, CdCmd: cdCmd, Actions: cfg.ActionList()})
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(a.stdout, script)
			return err
		},
	}
	cmd.Flags().StringVar(&cmdName, "cmd", "", "name of the open command (default: the editor's command, e.g. code)")
	cmd.Flags().StringVar(&cdCmd, "cd-cmd", defaultCdCmd, `name of the cd command ("" to skip it)`)
	cmd.Flags().BoolVar(&noCmd, "no-cmd", false, "do not define the open command")
	return cmd
}

func newRefreshCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Rescan the roots for projects now",
		Long: `Rescan the roots now. Rarely needed: the cache notices new and removed
projects on its own by watching folder modification times.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cache, err := a.projectCache()
			if err != nil {
				return err
			}
			start := time.Now()
			projects, fresh := cache.RefreshWithin(diagnosticBudget)
			if !fresh {
				return fmt.Errorf("scan did not finish within %s; a root may be on an unreachable drive (see `codehop doctor`)", diagnosticBudget)
			}
			fmt.Fprintf(a.stdout, "Found %d projects in %s.\n", len(projects), time.Since(start).Round(time.Millisecond))
			return nil
		},
	}
}

func newConfigCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show, create or edit the config file",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print the config file path",
			Args:  cobra.NoArgs,
			Run: func(*cobra.Command, []string) {
				fmt.Fprintln(a.stdout, paths.ConfigFile())
			},
		},
		&cobra.Command{
			Use:   "init",
			Short: "Write a commented config file with the defaults",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				path, created, err := writeConfigTemplate()
				if err != nil {
					return err
				}
				if !created {
					return fmt.Errorf("%s already exists", path)
				}
				fmt.Fprintf(a.stdout, "Wrote %s\n", path)
				return nil
			},
		},
		&cobra.Command{
			Use:   "edit",
			Short: "Open the config file in the editor (created if missing)",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				path, _, err := writeConfigTemplate()
				if err != nil {
					return err
				}
				return a.passthrough([]string{path})
			},
		},
	)
	return cmd
}

// writeConfigTemplate creates the config file from the template unless it
// exists. It reports the path and whether it created the file.
func writeConfigTemplate() (string, bool, error) {
	path := paths.ConfigFile()
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return path, false, err
	}
	if err := paths.WriteFileAtomic(path, []byte(config.Template)); err != nil {
		return path, false, err
	}
	return path, true, nil
}
