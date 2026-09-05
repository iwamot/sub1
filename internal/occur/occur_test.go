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
		{"old block uses CRLF", "a\nb\n", "a\r\nb", "near line 1: the old block uses CRLF line endings"},
		{"file has trailing whitespace", "x\na \nb\n", "a\nb", "near line 2: file line 2 has trailing whitespace"},
		{"old block has trailing whitespace", "x\na\nb\n", "a\nb\t", "near line 2: old block line 2 has trailing whitespace"},
		{"trailing whitespace later in the block", "a\nb\t\nc\n", "a\nb\nc", "near line 1: file line 2 has trailing whitespace"},
		{"tabs versus spaces", "x\n\tfoo\n\tbar\n", "  foo\n  bar", "near line 2: file line 2 starts with 1 tab, old block line 1 with 2 spaces"},
		{"indentation differs later in the block", "foo\n    bar\n", "foo\n  bar", "near line 1: file line 2 starts with 4 spaces, old block line 2 with 2 spaces"},
		{"mixed and none", "\t  foo\n  bar\n", "foo\nbar", "near line 1: file line 1 starts with mixed tabs and spaces, old block line 1 with no indentation"},
		{"several tabs", "\t\tfoo\n\t\tbar\n", "\tfoo\n\tbar", "near line 1: file line 1 starts with 2 tabs, old block line 1 with 1 tab"},
		{"whitespace hint reports every match", "\tfoo\nx\n\tfoo\n", "  foo", "near lines 1, 3: file line 1 starts with 1 tab, old block line 1 with 2 spaces"},
		{"CRLF and tabs", "build:\r\n\tgo build\r\n", "build:\n    go build", "near line 1: the file uses CRLF line endings; file line 2 starts with 1 tab, old block line 2 with 4 spaces"},
		{"CRLF and trailing whitespace", "a \r\nb\r\n", "a\nb", "near line 1: the file uses CRLF line endings; file line 1 has trailing whitespace"},
		{"old block CRLF and tabs", "\tfoo\n\tbar\n", "  foo\r\n  bar", "near line 1: the old block uses CRLF line endings; file line 1 starts with 1 tab, old block line 1 with 2 spaces"},
		{"prefix", "x\na\nb\nc\nd\n", "a\nb\nC\nd", "near line 2: first 2 of 4 lines match; line 3 differs"},
		{"prefix keeps the longest", "a\nb\nX\n\na\nb\nc\nX\n", "a\nb\nc\nd", "near line 5: first 3 of 4 lines match; line 4 differs"},
		{"prefix keeps the earliest on a tie", "a\nb\nX\na\nb\nY\n", "a\nb\nc", "near line 1: first 2 of 3 lines match; line 3 differs"},
		{"prefix line must match whole", "a\nb\ncd\n", "a\nb\nc\ne", "near line 1: first 2 of 4 lines match; line 3 differs"},
		{"prefix stops at end of file", "a\nb", "a\nb\nc", "near line 1: first 2 of 3 lines match; line 3 differs"},
		{"prefix of one line says nothing", "a\nx\n", "a\nb", ""},
		{"prefix first line absent", "x\ny\n", "a\nb", ""},
		{"single-line old has no prefix", "abc\n", "abd", ""},
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
