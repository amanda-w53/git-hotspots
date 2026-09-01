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
		"-M",
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

	// stats is keyed by every path name a file has ever been known by, so a
	// lookup under either its old or new name (after a rename) lands on the
	// same *FileStat. Since git log walks newest-first, by the time we see a
	// commit that renamed old -> new, "new" is already the row's Path (it's
	// the name closer to HEAD); we then alias "old" to that same row so
	// older commits mentioning "old" keep accumulating into it.
	stats := make(map[string]*FileStat)
	seenInCommit := make(map[*FileStat]bool)

	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, commitMarker) {
			seenInCommit = make(map[*FileStat]bool)
			continue
		}

		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		oldPath, newPath, renamed := splitRenamePath(fields[2])

		fs, ok := stats[newPath]
		if !ok {
			fs = &FileStat{Path: newPath}
			stats[newPath] = fs
		}
		if renamed {
			stats[oldPath] = fs
		}

		if seenInCommit[fs] {
			// A commit can list the same path twice for merge conflicts
			// resolved oddly; only count it once per commit.
			continue
		}
		seenInCommit[fs] = true

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

// splitRenamePath takes the third numstat field and, if it describes a
// rename, returns the old and new paths with renamed set to true. With -M
// enabled git prints renames two ways: as a full "old/path.go => new/path.go"
// when nothing in the path is shared, or with the common prefix/suffix
// factored out as "shared/{old => new}/path.go" when only part of it
// changed. Anything else is returned unchanged with renamed set to false.
func splitRenamePath(s string) (oldPath, newPath string, renamed bool) {
	if open := strings.Index(s, "{"); open != -1 {
		end := strings.Index(s[open:], "}")
		if end == -1 {
			return s, s, false
		}
		end += open
		parts := strings.SplitN(s[open+1:end], " => ", 2)
		if len(parts) != 2 {
			return s, s, false
		}
		prefix, suffix := s[:open], s[end+1:]
		return prefix + parts[0] + suffix, prefix + parts[1] + suffix, true
	}
	if parts := strings.SplitN(s, " => ", 2); len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return s, s, false
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
