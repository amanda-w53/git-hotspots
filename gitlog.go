package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
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
// change counts across the matching commits. author, if non-empty, is
// passed straight through to git's --author, which matches as a regex
// against the commit's author name and email. exclude holds glob patterns
// (see matchesExclude) for paths to drop from the results entirely, such as
// vendored or generated code.
func collectStats(repoDir, since, pathspec, author string, exclude []string) (map[string]*FileStat, error) {
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
	if author != "" {
		args = append(args, "--author="+author)
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

	stats, err := parseNumstatLog(out, exclude)
	if err != nil {
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

// parseNumstatLog reads the output of `git log --numstat
// --pretty=format:<commitMarker>%H` (as produced by collectStats) and
// aggregates per-file change counts, dropping any path matched by exclude.
// Split out from collectStats so it can be exercised directly against a
// fixture log in tests, without needing a real git repository on disk.
func parseNumstatLog(r io.Reader, exclude []string) (map[string]*FileStat, error) {
	// stats is keyed by every path name a file has ever been known by, so a
	// lookup under either its old or new name (after a rename) lands on the
	// same *FileStat. Since git log walks newest-first, by the time we see a
	// commit that renamed old -> new, "new" is already the row's Path (it's
	// the name closer to HEAD); we then alias "old" to that same row so
	// older commits mentioning "old" keep accumulating into it.
	stats := make(map[string]*FileStat)
	seenInCommit := make(map[*FileStat]bool)

	scanner := bufio.NewScanner(r)
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
		// A file is excluded (or not) based on where it lives now, matching
		// the same "current path wins" rule used for merging rename history.
		if matchesExclude(newPath, exclude) {
			continue
		}

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

// matchesExclude reports whether path should be dropped based on patterns
// (as given to -exclude). A pattern with no "/" is matched against every
// path component, so "vendor" or "*.pb.go" exclude a directory or a file
// extension wherever it appears in the tree. A pattern ending in "/*" also
// matches everything below that directory, not just its direct children
// (path/filepath.Match's "*" alone stops at the next "/", which would miss
// vendor/pkg/sub/file.go for a pattern like "vendor/*"). Any other pattern
// containing "/" is matched against the full path with path/filepath.Match.
func matchesExclude(path string, patterns []string) bool {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		if !strings.Contains(pat, "/") {
			for _, part := range strings.Split(path, "/") {
				if ok, _ := filepath.Match(pat, part); ok {
					return true
				}
			}
			continue
		}
		if dir := strings.TrimSuffix(pat, "/*"); dir != pat && strings.HasPrefix(path, dir+"/") {
			return true
		}
		if ok, _ := filepath.Match(pat, path); ok {
			return true
		}
	}
	return false
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
