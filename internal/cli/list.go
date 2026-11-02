package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/match"
	"github.com/ardacoskun/codehop/internal/paths"
)

type listFormat struct {
	names, describe, paths, json bool
}

func newListCmd(a *app) *cobra.Command {
	var (
		f     listFormat
		query string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects, most frecent first",
		Long: `List discovered projects, most frecent first.

With --query, projects whose name starts with the query are returned; if
there are none, fuzzy matches are returned instead. Shell completion uses this.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// list feeds shell completion, so it must answer quickly.
			projects, err := a.projectsWithin(completionBudget)
			if err != nil {
				return err
			}
			return a.printList(a.completionCandidates(query, projects), f)
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&f.names, "names", false, "print only project names")
	fl.BoolVar(&f.describe, "describe", false, "print name<TAB>category lines (for shell completion)")
	fl.BoolVar(&f.paths, "paths", false, "print only absolute paths")
	fl.BoolVar(&f.json, "json", false, "print JSON")
	fl.StringVarP(&query, "query", "q", "", "filter by name (prefix matches first, then fuzzy)")
	cmd.MarkFlagsMutuallyExclusive("names", "describe", "paths", "json")
	return cmd
}

// completionCandidates returns projects for completing q: all projects when
// q is empty, prefix matches when there are any, fuzzy matches otherwise.
func (a *app) completionCandidates(q string, projects []discover.Project) []discover.Project {
	frecency := a.frecency()
	sorted := match.SortByFrecency(projects, frecency)
	if q == "" {
		return sorted
	}
	lower := strings.ToLower(q)
	var prefix []discover.Project
	for _, p := range sorted {
		if strings.HasPrefix(strings.ToLower(p.Key), lower) || strings.HasPrefix(strings.ToLower(p.Name), lower) {
			prefix = append(prefix, p)
		}
	}
	if len(prefix) > 0 {
		return prefix
	}
	res := match.Rank(q, projects, frecency)
	out := make([]discover.Project, len(res.Candidates))
	for i, c := range res.Candidates {
		out[i] = c.Project
	}
	return out
}

// completeProjects provides project names to cobra's own shell completion.
func (a *app) completeProjects(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	projects, err := a.projectsWithin(completionBudget)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var out []string
	for _, p := range a.completionCandidates(toComplete, projects) {
		out = append(out, p.Key+"\t"+describe(p))
	}
	return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
}

// describe is the short label shown next to a project in completion menus.
func describe(p discover.Project) string {
	if c := p.Category(); c != "" {
		return c
	}
	return filepath.Base(p.Root)
}

func (a *app) printList(projects []discover.Project, f listFormat) error {
	switch {
	case f.names:
		for _, p := range projects {
			fmt.Fprintln(a.stdout, p.Key)
		}
	case f.describe:
		for _, p := range projects {
			fmt.Fprintf(a.stdout, "%s\t%s\n", p.Key, describe(p))
		}
	case f.paths:
		for _, p := range projects {
			fmt.Fprintln(a.stdout, p.Path)
		}
	case f.json:
		type item struct {
			discover.Project
			Frecency float64 `json:"frecency"`
		}
		frecency := a.frecency()
		items := make([]item, len(projects))
		for i, p := range projects {
			items[i] = item{Project: p, Frecency: frecency(p.Path)}
		}
		enc := json.NewEncoder(a.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	default:
		t := newTable(a.stdout, colorEnabled(a.stdout))
		for _, p := range projects {
			t.row(cell{text: p.Key, style: styleBold}, cell{text: paths.Pretty(p.Path), style: styleDim})
		}
		t.flush()
	}
	return nil
}
