package main

import (
	"strings"
	"testing"
)

// fixtureLog mimics the output of
// `git log --no-merges -M --numstat --pretty=format:@@commit@@%H` for a
// small made-up history, newest commit first (as git log produces it):
//
//   c4: rename src/util.go -> src/helpers/util.go
//   c3: touch src/main.go and a binary asset
//   c2: touch src/util.go twice (simulating a merge-conflict duplicate line)
//   c1: add src/main.go and src/util.go
const fixtureLog = `@@commit@@c4
0	0	src/{ => helpers}/util.go

@@commit@@c3
5	1	src/main.go
-	-	assets/logo.png

@@commit@@c2
2	0	src/util.go
2	0	src/util.go

@@commit@@c1
10	0	src/main.go
8	0	src/util.go
`

func TestParseNumstatLog(t *testing.T) {
	stats, err := parseNumstatLog(strings.NewReader(fixtureLog), nil)
	if err != nil {
		t.Fatalf("parseNumstatLog: %v", err)
	}

	util, ok := stats["src/helpers/util.go"]
	if !ok {
		t.Fatalf("expected renamed path %q in stats, got keys %v", "src/helpers/util.go", keys(stats))
	}
	// c4 (rename, 0/0), c2 (counted once despite two numstat lines, 2/0),
	// c1 (8/0) => 3 commits, 10 added, 0 deleted.
	if util.Commits != 3 {
		t.Errorf("util.go Commits = %d, want 3", util.Commits)
	}
	if util.Added != 10 || util.Deleted != 0 {
		t.Errorf("util.go Added/Deleted = %d/%d, want 10/0", util.Added, util.Deleted)
	}

	// The old path must alias to the same row as the new one, so a lookup
	// by the pre-rename name reflects the full history too.
	if old := stats["src/util.go"]; old != util {
		t.Errorf("stats[%q] does not alias to the renamed row", "src/util.go")
	}

	main, ok := stats["src/main.go"]
	if !ok {
		t.Fatalf("expected %q in stats", "src/main.go")
	}
	if main.Commits != 2 {
		t.Errorf("main.go Commits = %d, want 2", main.Commits)
	}
	if main.Added != 15 || main.Deleted != 1 {
		t.Errorf("main.go Added/Deleted = %d/%d, want 15/1", main.Added, main.Deleted)
	}

	logo, ok := stats["assets/logo.png"]
	if !ok {
		t.Fatalf("expected %q in stats", "assets/logo.png")
	}
	if logo.Commits != 1 {
		t.Errorf("logo.png Commits = %d, want 1", logo.Commits)
	}
	if logo.Added != 0 || logo.Deleted != 0 {
		t.Errorf("logo.png Added/Deleted = %d/%d, want 0/0 for a binary file", logo.Added, logo.Deleted)
	}
}

func TestParseNumstatLogExclude(t *testing.T) {
	stats, err := parseNumstatLog(strings.NewReader(fixtureLog), []string{"assets/*"})
	if err != nil {
		t.Fatalf("parseNumstatLog: %v", err)
	}
	if _, ok := stats["assets/logo.png"]; ok {
		t.Errorf("expected %q to be excluded, but it was present", "assets/logo.png")
	}
	if _, ok := stats["src/main.go"]; !ok {
		t.Errorf("expected %q to remain, exclude pattern should not have matched it", "src/main.go")
	}
}

func TestMatchesExclude(t *testing.T) {
	cases := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"vendor/lib/thing.go", []string{"vendor"}, true},
		{"src/vendored.go", []string{"vendor"}, false},
		{"vendor/lib/thing.go", []string{"vendor/*"}, true},
		{"vendor/lib/deep/thing.go", []string{"vendor/*"}, true},
		{"gen/api.pb.go", []string{"*.pb.go"}, true},
		{"gen/api.go", []string{"*.pb.go"}, false},
		{"internal/generated/api.go", []string{"generated"}, true},
		{"src/main.go", []string{"vendor", "*.pb.go"}, false},
		{"src/main.go", nil, false},
	}
	for _, c := range cases {
		if got := matchesExclude(c.path, c.patterns); got != c.want {
			t.Errorf("matchesExclude(%q, %v) = %v, want %v", c.path, c.patterns, got, c.want)
		}
	}
}

func TestSplitRenamePath(t *testing.T) {
	cases := []struct {
		in          string
		oldP, newP  string
		wantRenamed bool
	}{
		{"src/main.go", "src/main.go", "src/main.go", false},
		{"src/{ => helpers}/util.go", "src/util.go", "src/helpers/util.go", true},
		{"{old => new}.go", "old.go", "new.go", true},
		{"pkg/a.go => pkg/b.go", "pkg/a.go", "pkg/b.go", true},
	}
	for _, c := range cases {
		oldP, newP, renamed := splitRenamePath(c.in)
		if oldP != c.oldP || newP != c.newP || renamed != c.wantRenamed {
			t.Errorf("splitRenamePath(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.in, oldP, newP, renamed, c.oldP, c.newP, c.wantRenamed)
		}
	}
}

func keys(m map[string]*FileStat) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
