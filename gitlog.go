package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// commitMarker prefixes each commit hash line in the log output so we can
// tell it apart from the numstat lines that follow it. Chosen because it
// can't appear at the start of a numstat line (those always start with a
// digit or a dash).
const commitMarker = "@@commit@@"

// FileStat aggregates how often a path changed and by how much.
type FileStat struct {
	Path    string
	Commits int
	Added   int
	Deleted int
}

// collectStats runs `git log --numstat` in repoDir and aggregates per-file
// change counts across the matching commits.
func collectStats(repoDir, since, pathspec string) (map[string]*FileStat, error) {
	args := []string{
		"-C", repoDir,
		"log",
		"--no-merges",
		"--numstat",
		"--pretty=format:" + commitMarker + "%H",
	}
	if since != "" {
		args = append(args, "--since="+since)
	}
	if pathspec != "" {
		args = append(args, "--", pathspec)
	}

	cmd := exec.Command("git", args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	stats := make(map[string]*FileStat)
	seenInCommit := make(map[string]bool)

	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, commitMarker) {
			seenInCommit = make(map[string]bool)
			continue
		}

		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		path := fields[2]
		if seenInCommit[path] {
			// A commit can list the same path twice for merge conflicts
			// resolved oddly; only count it once per commit.
			continue
		}
		seenInCommit[path] = true

		fs, ok := stats[path]
		if !ok {
			fs = &FileStat{Path: path}
			stats[path] = fs
		}
		fs.Commits++
		fs.Added += parseNumstatField(fields[0])
		fs.Deleted += parseNumstatField(fields[1])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("git log: %s", msg)
		}
		return nil, fmt.Errorf("git log: %w", err)
	}

	return stats, nil
}

// parseNumstatField turns a numstat count into an int. Binary files report
// "-" instead of a number; we treat that as zero since line counts don't
// apply, but the file still counted toward Commits above.
func parseNumstatField(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
