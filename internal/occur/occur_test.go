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
