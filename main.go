// git-hotspots ranks the files in a git repository by how often they
// change, so you can find the handful of files that absorb most of the
// churn (and, usually, most of the bugs and merge conflicts).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

type jsonRow struct {
	Path    string `json:"path"`
	Commits int    `json:"commits"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Churn   int    `json:"churn"`
}

func main() {
	repoDir := flag.String("repo", ".", "path to the git repository")
	since := flag.String("since", "", `only consider commits after this date (anything "git log --since" accepts, e.g. "6 months ago")`)
	pathspec := flag.String("path", "", "limit to commits touching this path (a git pathspec)")
	author := flag.String("author", "", "only consider commits by an author matching this pattern (regex, matched against name and email)")
	limit := flag.Int("limit", 20, "number of files to show")
	jsonOut := flag.Bool("json", false, "print machine-readable JSON instead of a table")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "git-hotspots: find the most frequently changed files in a git repo\n\n")
		fmt.Fprintf(os.Stderr, "usage: %s [flags]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *limit <= 0 {
		fmt.Fprintln(os.Stderr, "git-hotspots: -limit must be positive")
		os.Exit(2)
	}

	stats, err := collectStats(*repoDir, *since, *pathspec, *author)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-hotspots: %v\n", err)
		os.Exit(1)
	}

	rows := make([]*FileStat, 0, len(stats))
	for _, fs := range stats {
		rows = append(rows, fs)
	}
	sort.Slice(rows, func(i, j int) bool {
		ci, cj := rows[i].Commits, rows[j].Commits
		if ci != cj {
			return ci > cj
		}
		return rows[i].Path < rows[j].Path
	})
	if len(rows) > *limit {
		rows = rows[:*limit]
	}

	if *jsonOut {
		printJSON(rows)
		return
	}
	printTable(rows)
}

func printJSON(rows []*FileStat) {
	out := make([]jsonRow, 0, len(rows))
	for _, fs := range rows {
		out = append(out, jsonRow{
			Path:    fs.Path,
			Commits: fs.Commits,
			Added:   fs.Added,
			Deleted: fs.Deleted,
			Churn:   fs.Added + fs.Deleted,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "git-hotspots: %v\n", err)
		os.Exit(1)
	}
}

func printTable(rows []*FileStat) {
	if len(rows) == 0 {
		fmt.Println("no commits matched")
		return
	}
	fmt.Printf("%-8s %-8s %-8s  %s\n", "COMMITS", "+LINES", "-LINES", "PATH")
	for _, fs := range rows {
		fmt.Printf("%-8d %-8d %-8d  %s\n", fs.Commits, fs.Added, fs.Deleted, fs.Path)
	}
}
