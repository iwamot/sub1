# sub1

[![release](https://img.shields.io/github/v/release/iwamot/sub1)](https://github.com/iwamot/sub1/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/iwamot/sub1)](https://pkg.go.dev/github.com/iwamot/sub1)

Replace a literal text block in a file, exactly once. Built for coding agents that edit through the shell.

```
$ sub1 app.py <<'SUB1'
    if reply is None:
        return
====
    if reply is None:
        return None
====
SUB1
app.py: replaced at line 42
```

The old and new blocks come from a heredoc, so nothing needs escaping. The file is rewritten only when the old block occurs exactly once. Otherwise it is left alone, the count is reported, and the exit code is 1.

## Why

An agent that edits through a shell has two common tools, and both are bad at it.

`sed -i` succeeds even when the old text is not in the file, so a missed edit goes unnoticed. Multi-line text and characters like `/`, `&`, and `$` need escaping, which is where edits go wrong.

A Python one-off that reads the file, asserts the old text occurs once, replaces it, and writes it back does the right thing. But the agent re-types it for every edit. In one month of the author's Claude Code sessions, that script was written by hand more than 700 times.

`sub1` is that script, made into a command.

## Setup

Install it where the agent runs:

```bash
brew install iwamot/tap/sub1
```

Or with Go:

```bash
go install github.com/iwamot/sub1@latest
```

Or download a prebuilt binary from the [Releases page](https://github.com/iwamot/sub1/releases).

Then tell the agent to use it, in `CLAUDE.md`, `AGENTS.md`, or whichever file your agent reads:

```markdown
To replace part of a file from the shell, use `sub1` instead of sed or an ad-hoc script: `sub1 FILE <<'SUB1'`, then the old lines, a line `====`, the new lines, another line `====`, and `SUB1`. Before writing the heredoc, check both blocks: if a line is exactly `SUB1`, use another terminator; if a line is exactly `====`, pass `-d SEP` and write SEP on both separator lines instead. The file is rewritten only when the old block occurs exactly once. On exit 1, read the reported count and lines. When the count is 0, a second line, if present, says how the old block differs from the file: fix the old block, or read the file again if there is no second line. Otherwise widen the old block until it is unique, or pass `-n N` for the number of occurrences you expect.
```

That paragraph is all the agent needs. `sub1 --instructions` prints the same paragraph, for setup scripts and machines where this page is not at hand.

## What the agent sees

Renaming a function. The definition is unique, so the edit lands and the output is one line:

```
$ sub1 handlers.py <<'SUB1'
def _tool_chunks(chunks):
====
def tool_chunks(chunks):
====
SUB1
handlers.py: replaced at line 12
```

The call sites are not unique. Nothing is written, and the count comes back with the lines:

```
$ sub1 handlers.py <<'SUB1'
_tool_chunks(
====
tool_chunks(
====
SUB1
sub1: handlers.py: old block found 3 times (lines 27, 40, 41), expected 1
```

The agent then either widens the old block until it is unique, or says how many occurrences it expects. The count is still checked, so a stray extra match would still stop the edit:

```
$ sub1 -n 3 handlers.py <<'SUB1'
_tool_chunks(
====
tool_chunks(
====
SUB1
handlers.py: replaced at lines 27, 40, 41
```

The old block was typed with spaces, but the file is indented with tabs. Nothing matches, and the closest place is described so that the old block can be fixed without reading the file again:

```
$ sub1 Makefile <<'SUB1'
build:
    go build ./...
====
build:
    go build -trimpath ./...
====
SUB1
sub1: Makefile: old block found 0 times, expected 1
  near line 3: file line 4 starts with 1 tab, old block line 2 with 4 spaces
```

The second line is a guess at what went wrong. The count on the first line is what decided that nothing was replaced.

Deleting a line. The new block is empty, so the two `====` lines follow each other:

```
$ sub1 handlers.py <<'SUB1'
import os
====
====
SUB1
handlers.py: replaced at line 1
```

## Reference

```
$ sub1 --help
sub1 — replace a literal text block in a file, exactly once.

Usage:
  sub1 [-n N] [-d SEP] [--] FILE <<'SUB1'
  old text (one or more lines)
  ====
  new text (zero or more lines)
  ====
  SUB1

OLD and NEW come from stdin, each followed by a line equal to SEP. The final
newline of each block is dropped. FILE is rewritten only when OLD occurs
exactly N times; otherwise it is left untouched.

Options:
  -n N            expect OLD N times (default 1)
  -d SEP          line that ends OLD and ends NEW (default "====")
  -h, --help      show this help
  -v, --version   show the version
  --instructions  print the paragraph for the agent's instruction file

Exit codes:
  0  replaced
  1  OLD found a different number of times than expected
  2  usage error, or FILE could not be read or written
```

- Matching is byte-exact: tabs and spaces differ. Line endings are the one exception: when every line of the file ends with CRLF, both blocks are read as CRLF too, so a heredoc can edit such a file, and the summary notes `(CRLF)`. A file that mixes CRLF and LF is matched as is.
- Because matching is byte-exact, the old block can match part of a line, and the summary then carries a note in parentheses, several joined with `;`. `1 match starts inside an identifier` and `1 match ends inside an identifier` say that the byte next to the match and the byte of the old block beside it are both identifier characters, as when `x = 1` is found in `max = 1`. `1 match starts mid-line with a multi-line new block` says that the lines of the new block after the first were inserted with the indentation they were written with, which is not the indentation of the line the match started in. `the file already contained the new block` says that the new block contains the old one and was already in the file, so the edit repeated the rest of it, which is what running the same edit twice looks like. A note is a fact about where the match sits, not an error: renaming an identifier at its call sites matches inside lines on purpose. Check the edit when a note was not expected.
- An empty new block deletes the old block. When the old block is a whole line or a run of whole lines, the line break after it goes too, so the lines disappear instead of leaving a blank line behind. An old block that starts or ends in the middle of a line leaves that line's break where it is, and an old block that ends with a blank line takes nothing beyond it. Deleting a last line that has no line break leaves the file ending with the line break of the line before it.
- The closing `====` line is required. A heredoc ends at the first line equal to its terminator, so a terminator written on the separator line by mistake, or one that also appears inside a block, ends the heredoc early. The input that reaches `sub1` then stops before the closing line, and it is rejected instead of being applied as if it were whole.
- If either block contains a line that is exactly `====` (a Markdown setext underline, or a quoted `sub1` call, for example), pass another separator with `-d`.
- The file's permissions are kept. Only its contents change.
- The file is never left half-written. The new contents go to a temporary `.sub1-*` file next to it, which is then renamed into place. If `sub1` is interrupted, the file still has either the old or the new contents, and any leftover `.sub1-*` file can be deleted.
- Because of that rename, the directory must be writable, and hard links to the file are not preserved.

## Out of scope

- Regular expressions. The blocks are literal bytes.
- Editing more than one file per call. Run `sub1` once per file.
- Reading the text to edit from stdin. stdin carries the blocks; the file to edit is named on the command line and rewritten in place.
- Ranges such as "from this line to that line". `sub1` only replaces text it was shown, so it never removes more than you wrote in the old block, plus the one line break that follows each occurrence when an empty new block deletes whole lines.

## License

MIT
