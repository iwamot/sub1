package occur

import (
	"reflect"
	"testing"
)

func TestLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		want    []int
	}{
		{"absent", "a\nb\n", "z", nil},
		{"first line", "foo\nbar\n", "foo", []int{1}},
		{"later line", "a\nfoo\nb\n", "foo", []int{2}},
		{"mid-line match reports its line", "a\n  x = foo()\n", "foo", []int{2}},
		{"several occurrences", "x(1)\ny\nx(2)\nx(3)\n", "x(", []int{1, 3, 4}},
		{"two on one line count separately", "ab ab\n", "ab", []int{1, 1}},
		{"multi-line block", "a\nb\nc\nb\nc\n", "b\nc", []int{2, 4}},
		{"overlapping candidates are not double counted", "aaa\n", "aa", []int{1}},
		{"no trailing newline", "abc", "c", []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines([]byte(tt.content), []byte(tt.old))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Lines = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplace(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		new     string
		want    string
	}{
		{"absent", "a\nb\n", "z", "y", "a\nb\n"},
		{"replaces every occurrence", "x(1)\ny\nx(2)\n", "x(", "z(", "z(1)\ny\nz(2)\n"},
		{"replacement is literal", "a\nb\n", "b", "\n", "a\n\n\n"},
		{"overlapping candidates are not double replaced", "aaa\n", "aa", "b", "ba\n"},

		{"empty new deletes the whole line", "a\nb\nc\n", "b", "", "a\nc\n"},
		{"empty new deletes several whole lines", "a\nb\nc\nd\n", "b\nc", "", "a\nd\n"},
		{"empty new deletes the first line", "b\nc\n", "b", "", "c\n"},
		{"empty new deletes a CRLF line", "a\r\nb\r\nc\r\n", "b", "", "a\r\nc\r\n"},
		{"empty new deletes the line break the file has", "a\r\nb\nc\r\n", "b", "", "a\r\nc\r\n"},
		{"empty new deletes the line break the file has, the other way", "a\nb\r\nc\n", "b", "", "a\nc\n"},
		{"empty new deletes a line that starts with a blank line", "a\n\nb\nc\n", "\nb", "", "a\nc\n"},
		{"empty new deletes each occurrence on its own", "b\nab\nb\n", "b", "", "a\n"},
		{"empty new deletes the whole file", "b\n", "b", "", ""},
		{"empty new deletes a last line without a line break", "a\nb", "b", "", "a\n"},

		{"empty new keeps the line break of a mid-line old", "ab\ncd\n", "b", "", "a\ncd\n"},
		{"empty new keeps the line break when old ends a line", "ab\ncd\n", "b\ncd", "", "a\n"},
		{"empty new keeps the line break of adjacent mid-line occurrences", "bb\n", "b", "", "\n"},
		{"empty new keeps a blank line after an old that ends with one", "a\nb\n\nc\n", "b\n", "", "a\n\nc\n"},
		{"empty new keeps a blank line after a multi-line old that ends with one", "a\nb\nc\n\nd\n", "b\nc\n", "", "a\n\nd\n"},
		{"empty new keeps the line break of a CRLF old", "a\r\nb\r\n\r\nc\r\n", "b\r\n", "", "a\r\n\r\nc\r\n"},
		{"empty new keeps a lone CR", "a\nb\rc\n", "b", "", "a\n\rc\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Replace([]byte(tt.content), []byte(tt.old), []byte(tt.new))
			if string(got) != tt.want {
				t.Errorf("Replace = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMismatch(t *testing.T) {
	tests := []struct {
		name     string
		lines    []int
		expected int
		want     string
	}{
		{"none", nil, 1, "f.txt: old block found 0 times, expected 1"},
		{"one but wanted more", []int{7}, 2, "f.txt: old block found once (line 7), expected 2"},
		{"many", []int{1, 3, 4}, 1, "f.txt: old block found 3 times (lines 1, 3, 4), expected 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mismatch("f.txt", tt.lines, tt.expected); got != tt.want {
				t.Errorf("Mismatch = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHint(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		want    string
	}{
		{"nothing close", "a\nb\n", "z", ""},
		{"file uses CRLF", "a\r\nb\r\n", "a\nb", "near line 1: the file uses CRLF line endings"},
		{"file mixes CRLF and LF", "a\r\nb\nc\r\n", "a\nb\nc", "near line 1: the file has mixed line endings"},
		{"mixed line endings and tabs", "x\n\tfoo\r\n\tbar\n", "  foo\n  bar", "near line 2: the file has mixed line endings; file line 2 starts with 1 tab, old block line 1 with 2 spaces"},
		{"old block CRLF gets no hint", "a\nb\n", "a\r\nb", ""},
		{"file has trailing whitespace", "x\na \nb\n", "a\nb", "near line 2: file line 2 has trailing whitespace"},
		{"old block has trailing whitespace", "x\na\nb\n", "a\nb\t", "near line 2: old block line 2 has trailing whitespace"},
		{"trailing whitespace later in the block", "a\nb\t\nc\n", "a\nb\nc", "near line 1: file line 2 has trailing whitespace"},
		{"tabs versus spaces", "x\n\tfoo\n\tbar\n", "  foo\n  bar", "near line 2: file line 2 starts with 1 tab, old block line 1 with 2 spaces"},
		{"indentation differs later in the block", "foo\n    bar\n", "foo\n  bar", "near line 1: file line 2 starts with 4 spaces, old block line 2 with 2 spaces"},
		{"mixed and none", "\t  foo\n  bar\n", "foo\nbar", "near line 1: file line 1 starts with mixed tabs and spaces, old block line 1 with no indentation"},
		{"several tabs", "\t\tfoo\n\t\tbar\n", "\tfoo\n\tbar", "near line 1: file line 1 starts with 2 tabs, old block line 1 with 1 tab"},
		{"whitespace hint reports every match", "\tfoo\nx\n\tfoo\n", "  foo", "near lines 1, 3: file line 1 starts with 1 tab, old block line 1 with 2 spaces"},
		{"inner spaces", "a  b\nc\n", "a b\nc", "near line 1: file line 1 has 2 spaces where the old block has 1 space"},
		{"inner tab versus space", "x\na\tb\n", "a b", "near line 2: file line 2 has 1 tab where the old block has 1 space"},
		{"inner difference later in the block", "x\na b\n", "x\na  b", "near line 1: file line 2 has 1 space where the old block has 2 spaces"},
		{"inner difference after an equal run", "a b  c\n", "a b c", "near line 1: file line 1 has 2 spaces where the old block has 1 space"},
		{"inner mixed run", "a \t b\n", "a b", "near line 1: file line 1 has mixed tabs and spaces where the old block has 1 space"},
		{"leading and inner together", "\ta  b\n", "  a b", "near line 1: file line 1 has 1 tab where the old block has 2 spaces"},
		{"inner whitespace cannot be absent on one side", "a b\n", "ab", ""},
		{"CRLF and tabs", "build:\r\n\tgo build\r\n", "build:\n    go build", "near line 1: the file uses CRLF line endings; file line 2 starts with 1 tab, old block line 2 with 4 spaces"},
		{"CRLF and trailing whitespace", "a \r\nb\r\n", "a\nb", "near line 1: the file uses CRLF line endings; file line 1 has trailing whitespace"},

		{"run at the start of old", "x\na\nb\nc\nd\n", "a\nb\nC\nd", "near line 2: lines 1-2 of the old block match file lines 2-3; line 3 differs"},
		{"run keeps the longest", "a\nb\nX\n\na\nb\nc\nX\n", "a\nb\nc\nd", "near line 5: lines 1-3 of the old block match file lines 5-7; line 4 differs"},
		{"run keeps the earliest file place on a tie", "a\nb\nX\na\nb\nY\n", "a\nb\nc", "near line 1: lines 1-2 of the old block match file lines 1-2; line 3 differs"},
		{"run line must match whole", "a\nb\ncd\n", "a\nb\nc\ne", "near line 1: lines 1-2 of the old block match file lines 1-2; line 3 differs"},
		{"run first line must match whole too", "xfoo\nbar\nbaz\n", "foo\nbar\nqux", ""},
		{"run skips a first line that only ends a file line", "xa\nb\nX\na\nb\nY\n", "a\nb\nc", "near line 4: lines 1-2 of the old block match file lines 4-5; line 3 differs"},
		{"run stops at end of file", "a\nb", "a\nb\nc", "near line 1: lines 1-2 of the old block match file lines 1-2; line 3 differs"},
		{"run in a CRLF file", "a\r\nb\r\nX\r\n", "a\nb\nc", "near line 1: the file uses CRLF line endings; lines 1-2 of the old block match file lines 1-2; line 3 differs"},
		{"run of one line says nothing", "a\nx\n", "a\nb", ""},
		{"run first line absent and nothing else in a row", "x\ny\n", "a\nb", ""},
		{"single-line old has no run", "abc\n", "abd", ""},
		{"run at the end of old: first line differs", "a\nb\nc\nd\n", "x\nb\nc\nd", "near line 2: lines 2-4 of the old block match file lines 2-4; line 1 differs"},
		{"run in the middle of old", "p\nb\nc\nq\n", "a\nb\nc\nd", "near line 2: lines 2-3 of the old block match file lines 2-3; lines 1 and 4 differ"},
		{"run in the middle of old with more lines after", "p\nb\nc\nq\nr\n", "a\nb\nc\nd\ne\nf", "near line 2: lines 2-3 of the old block match file lines 2-3; lines 1 and 4 differ"},
		{"longer run later in old beats a shorter one at its start", "a\nX\nb\nc\nd\n", "a\nb\nc\nd\ne", "near line 3: lines 2-4 of the old block match file lines 3-5; lines 1 and 5 differ"},
		{"earliest file place wins a tie even when later in old", "b\nc\nX\na\nb\nX\n", "a\nb\nc", "near line 1: lines 2-3 of the old block match file lines 1-2; line 1 differs"},
		{"same file place, earliest old place wins", "a\nb\nX\n", "a\nb\nY\na\nb", "near line 1: lines 1-2 of the old block match file lines 1-2; line 3 differs"},
		{"run near the top of the file with old lines before it", "b\nc\nX\n", "a\nb\nc\nd", "near line 1: lines 2-3 of the old block match file lines 1-2; lines 1 and 4 differ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Lines([]byte(tt.content), []byte(tt.old)) != nil {
				t.Fatal("test case must not match as is")
			}
			if got := Hint([]byte(tt.content), []byte(tt.old)); got != tt.want {
				t.Errorf("Hint = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		name  string
		lines []int
		want  string
	}{
		{"one", []int{7}, "f.txt: replaced at line 7"},
		{"many", []int{1, 3, 4}, "f.txt: replaced at lines 1, 3, 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Summary("f.txt", tt.lines); got != tt.want {
				t.Errorf("Summary = %q, want %q", got, tt.want)
			}
		})
	}
}
