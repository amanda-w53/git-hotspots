# git-hotspots

Every repo has a handful of files that get touched constantly: a giant
router, a shared config, a "utils.go" that grew for three years. They're
usually the files most likely to have a bad merge conflict, and often the
ones with the least test coverage relative to how much they change.
`git-hotspots` finds them by walking `git log --numstat` and ranking files
by how many commits touched them.

It's a thin wrapper around information git already has. No network calls,
no caching, no external services — it just runs `git log` in the repo you
point it at and summarizes the output.

## Build

```
go build -o git-hotspots .
```

## Usage

Run it from anywhere, pointing at a repo:

```
git-hotspots -repo ~/code/some-project
```

```
COMMITS  +LINES   -LINES    PATH
142      3801     2960      src/server.go
88       1204     640       src/config.go
53       610      210       cmd/root.go
```

Narrow it down by time or by path:

```
git-hotspots -since "6 months ago" -limit 10
git-hotspots -path src/api -since "1 year ago"
```

Get JSON for scripting or feeding into another tool:

```
git-hotspots -json -limit 5
```

```json
[
  {
    "path": "src/server.go",
    "commits": 142,
    "added": 3801,
    "deleted": 2960,
    "churn": 6761
  }
]
```

## Flags

| Flag       | Default | Meaning                                             |
|------------|---------|------------------------------------------------------|
| `-repo`    | `.`     | Path to the git repository                           |
| `-since`   | (none)  | Only count commits after this date, e.g. `"3 months ago"` |
| `-path`    | (none)  | Restrict to commits touching this pathspec           |
| `-limit`   | `20`    | Number of files to print                             |
| `-json`    | `false` | Print JSON instead of a table                        |

## How it counts

For every non-merge commit, `git-hotspots` reads the numstat lines (added
lines, deleted lines, path) and increments a per-file commit counter once
per commit, plus running totals of lines added and deleted. Binary files
show up with a commit count but zero line counts, since git doesn't report
line diffs for them.

Rename detection is on (`-M`), and a renamed file's history before and
after the move is merged into a single row under its current path, instead
of splitting into an "old" row and a "new" row that both undercount how
often the file actually changes.

## License

MIT, see LICENSE.
