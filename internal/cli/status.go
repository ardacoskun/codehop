package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/gitstat"
	"github.com/ardacoskun/codehop/internal/match"
	"github.com/ardacoskun/codehop/internal/safetext"
)

func newStatusCmd(a *app) *cobra.Command {
	var dirtyOnly, asJSON bool
	cmd := &cobra.Command{
		Use:   "status [filter]",
		Short: "Show the git state of every project",
		Long: `Show branch, uncommitted changes, unpushed/unpulled commits and the last
commit of every project. An optional filter narrows the list like "open" does.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projects, err := a.projects()
			if err != nil {
				return err
			}
			if q := firstArg(args); q != "" {
				res := match.Rank(q, projects, nil)
				projects = projects[:0:0]
				for _, c := range res.Candidates {
					projects = append(projects, c.Project)
				}
			}
			sort.Slice(projects, func(i, j int) bool {
				return strings.ToLower(projects[i].Key) < strings.ToLower(projects[j].Key)
			})

			dirs := make([]string, len(projects))
			for i, p := range projects {
				dirs[i] = p.Path
			}
			statuses := gitstat.InspectAll(cmd.Context(), dirs)

			rows := make([]statusRow, 0, len(projects))
			for i, p := range projects {
				st := statuses[i]
				if dirtyOnly && (!st.IsRepo || (!st.Dirty() && st.Behind == 0)) {
					continue
				}
				rows = append(rows, statusRow{project: p, status: st})
			}
			if asJSON {
				return a.printStatusJSON(rows)
			}
			a.printStatusTable(rows)
			return nil
		},
		ValidArgsFunction: a.completeProjects,
	}
	cmd.Flags().BoolVarP(&dirtyOnly, "dirty", "d", false, "only projects with uncommitted, unpushed or unpulled work")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

type statusRow struct {
	project discover.Project
	status  gitstat.Status
}

func (a *app) printStatusTable(rows []statusRow) {
	if len(rows) == 0 {
		fmt.Fprintln(a.stdout, "Nothing to show.")
		return
	}
	now := a.now()
	t := newTable(a.stdout, colorEnabled(a.stdout))
	t.row(cell{"PROJECT", styleBold}, cell{"BRANCH", styleBold}, cell{"CHANGES", styleBold},
		cell{"SYNC", styleBold}, cell{"LAST COMMIT", styleBold})
	for _, r := range rows {
		st := r.status
		name := cell{text: r.project.Key}
		switch {
		case !st.IsRepo:
			t.row(name, cell{"-", styleDim}, cell{"not a git repo", styleDim}, cell{}, cell{})
		case st.Err != nil:
			t.row(name, cell{"?", styleRed}, cell{safetext.Sanitize(firstLine(st.Err.Error())), styleRed}, cell{}, cell{})
		default:
			t.row(name, cell{safetext.Sanitize(st.Branch), styleMagenta}, changesCell(st), syncCell(st), lastCommitCell(st, now))
		}
	}
	t.flush()
}

func changesCell(st gitstat.Status) cell {
	var parts []string
	if st.Changed > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", st.Changed))
	}
	if st.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", st.Untracked))
	}
	if len(parts) == 0 {
		return cell{"clean", styleGreen}
	}
	return cell{strings.Join(parts, ", "), styleYellow}
}

func syncCell(st gitstat.Status) cell {
	switch {
	case !st.HasUpstream:
		return cell{"no upstream", styleDim}
	case st.Ahead == 0 && st.Behind == 0:
		return cell{"up to date", styleGreen}
	}
	var parts []string
	if st.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", st.Ahead))
	}
	if st.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", st.Behind))
	}
	return cell{strings.Join(parts, " "), styleCyan}
}

func lastCommitCell(st gitstat.Status, now time.Time) cell {
	if st.LastCommit == nil {
		return cell{"no commits", styleDim}
	}
	return cell{gitstat.Age(st.LastCommit.Time, now) + " ago", styleDim}
}

func (a *app) printStatusJSON(rows []statusRow) error {
	type item struct {
		Key        string `json:"key"`
		Path       string `json:"path"`
		Repo       bool   `json:"repo"`
		Branch     string `json:"branch,omitempty"`
		Changed    int    `json:"changed"`
		Untracked  int    `json:"untracked"`
		Upstream   bool   `json:"upstream"`
		Ahead      int    `json:"ahead"`
		Behind     int    `json:"behind"`
		LastCommit string `json:"last_commit,omitempty"`
		Error      string `json:"error,omitempty"`
	}
	items := make([]item, len(rows))
	for i, r := range rows {
		st := r.status
		it := item{
			Key: r.project.Key, Path: r.project.Path, Repo: st.IsRepo, Branch: st.Branch,
			Changed: st.Changed, Untracked: st.Untracked, Upstream: st.HasUpstream,
			Ahead: st.Ahead, Behind: st.Behind,
		}
		if st.LastCommit != nil {
			it.LastCommit = st.LastCommit.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		if st.Err != nil {
			it.Error = st.Err.Error()
		}
		items[i] = it
	}
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
