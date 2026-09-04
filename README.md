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
To replace part of a file from the shell, use `sub1` instead of sed or an ad-hoc script: `sub1 FILE <<'SUB1'`, then the old lines, a line `====`, the new lines, and `SUB1`. It rewrites the file only when the old block occurs exactly once; on exit 1, read the reported count and either widen the old block or pass `-n N` for the number of occurrences you expect. If the text itself contains a `====` line, pass `-d` with another separator.
```

That paragraph is all the agent needs.

## What the agent sees

Renaming a function. The definition is unique, so the edit lands and the output is one line:

```
$ sub1 handlers.py <<'SUB1'
def _tool_chunks(chunks):
====
def tool_chunks(chunks):
SUB1
handlers.py: replaced at line 12
```

The call sites are not unique. Nothing is written, and the count comes back:

```
$ sub1 handlers.py <<'SUB1'
_tool_chunks(
====
tool_chunks(
SUB1
sub1: handlers.py: old block found 3 times, expected 1
```

The agent then either widens the old block until it is unique, or says how many occurrences it expects. The count is still checked, so a stray extra match would still stop the edit:

```
$ sub1 -n 3 handlers.py <<'SUB1'
_tool_chunks(
====
tool_chunks(
SUB1
handlers.py: replaced at lines 27, 40, 41
```

## Reference

```
$ sub1 --help
sub1 — replace a literal text block in a file, exactly once.

Usage:
  sub1 [-n N] [-d SEP] FILE <<'SUB1'
  old text (one or more lines)
  ====
  new text (zero or more lines)
  SUB1
  sub1 -h, --help
  sub1 -v, --version

OLD and NEW come from stdin, split at the single line equal to SEP (default
"===="). The final newline of each block is dropped. FILE is rewritten only
when OLD occurs exactly N times (default 1); otherwise it is left untouched.

Exit codes:
  0  replaced
  1  OLD found a different number of times than expected
  2  usage error, or FILE could not be read or written
```

- Matching is byte-exact: tabs and spaces differ, and a CRLF file needs CRLF in the old block.
- An empty new block deletes the old block. To remove a whole line including its line break, start the old block one line earlier and repeat that line in the new block.
- If either block contains a line that is exactly `====` (a Markdown setext underline, for example), pass another separator with `-d`.
- The file's permissions are kept. Only its contents change.

## Out of scope

- Regular expressions. The blocks are literal bytes.
- Editing more than one file per call. Run `sub1` once per file.
- Reading the text to edit from stdin. stdin carries the blocks; the file to edit is named on the command line and rewritten in place.
- Ranges such as "from this line to that line". `sub1` only replaces text it was shown, so it never removes more than you wrote in the old block.

## License

MIT
