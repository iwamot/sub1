package block

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	sep := []byte("====")
	tests := []struct {
		name    string
		input   string
		sep     []byte
		wantOld string
		wantNew string
		wantErr string
	}{
		{
			name:    "single lines",
			input:   "foo\n====\nbar\n====\n",
			sep:     sep,
			wantOld: "foo",
			wantNew: "bar",
		},
		{
			name:    "multi-line blocks keep inner newlines and indentation",
			input:   "  if x:\n    return\n====\n  if x:\n    return None\n====\n",
			sep:     sep,
			wantOld: "  if x:\n    return",
			wantNew: "  if x:\n    return None",
		},
		{
			name:    "empty new block deletes",
			input:   "gone\n====\n====\n",
			sep:     sep,
			wantOld: "gone",
			wantNew: "",
		},
		{
			name:    "no trailing newline after new block",
			input:   "a\n====\nb\n====",
			sep:     sep,
			wantOld: "a",
			wantNew: "b",
		},
		{
			name:    "blank line inside old block is kept",
			input:   "a\n\nb\n====\nc\n====\n",
			sep:     sep,
			wantOld: "a\n\nb",
			wantNew: "c",
		},
		{
			name:    "custom separator",
			input:   "Title\n====\n%%\nTitle\n----\n%%\n",
			sep:     []byte("%%"),
			wantOld: "Title\n====",
			wantNew: "Title\n----",
		},
		{
			name:    "closing separator must fill the whole line",
			input:   "a\n====\nb\n==== \n",
			sep:     sep,
			wantErr: "line 4 looks like \"====\" but has trailing whitespace; remove the spaces or tabs after it",
		},
		{
			name:    "middle separator must fill the whole line",
			input:   "a\n====\t\nb\n====\n",
			sep:     sep,
			wantErr: "line 2 looks like \"====\" but has trailing whitespace; remove the spaces or tabs after it",
		},
		{
			name:    "the first line that looks like a separator is the one named",
			input:   "a\n==== \nb\n==== \n",
			sep:     sep,
			wantErr: "line 2 looks like \"====\" but has trailing whitespace",
		},
		{
			name:    "old form without the closing separator",
			input:   "a\n====\nb\n",
			sep:     sep,
			wantErr: "input ended before the second \"====\" line; if a content line equals the heredoc terminator",
		},
		{
			name:    "old form deletion looks like a closing separator alone",
			input:   "a\n====\n",
			sep:     sep,
			wantErr: "only one \"====\" line; an empty new block still takes two \"====\" lines after the old block",
		},
		{
			name:    "shell cut the heredoc inside the old block",
			input:   "a\nb\n",
			sep:     sep,
			wantErr: "no \"====\" line found; if a content line equals the heredoc terminator",
		},
		{
			name:    "shell cut the heredoc after a quoted separator in the old block",
			input:   "a\n====\nb\nc\n",
			sep:     sep,
			wantErr: "input ended before the second \"====\" line",
		},
		{
			name:    "two separators before the closing one",
			input:   "a\n====\nb\n====\nc\n====\n",
			sep:     sep,
			wantErr: "found 3 \"====\" lines, expected 2; if a content line equals \"====\", pass -d SEP",
		},
		{
			name:    "closing separator alone",
			input:   "====\n",
			sep:     sep,
			wantErr: "only one \"====\" line",
		},
		{
			name:    "empty old block",
			input:   "====\nb\n====\n",
			sep:     sep,
			wantErr: "old block is empty; put at least one line before the first \"====\" line",
		},
		{
			name:    "identical blocks",
			input:   "same\n====\nsame\n====\n",
			sep:     sep,
			wantErr: "identical",
		},
		{
			name:    "shell cut the heredoc after the second separator",
			input:   "a\n====\nb\n====\nc\n",
			sep:     sep,
			wantErr: "input ended before the closing \"====\" line; if a content line equals",
		},
		{
			name:    "empty input",
			input:   "",
			sep:     sep,
			wantErr: "no input on stdin; pass the old and new blocks as a heredoc (see --help)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split([]byte(tt.input), tt.sep)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Split err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Split err = %v", err)
			}
			if string(got.Old) != tt.wantOld {
				t.Errorf("Old = %q, want %q", got.Old, tt.wantOld)
			}
			if string(got.New) != tt.wantNew {
				t.Errorf("New = %q, want %q", got.New, tt.wantNew)
			}
		})
	}
}
