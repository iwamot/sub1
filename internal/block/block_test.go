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
			wantErr: "closing \"====\" line is missing",
		},
		{
			name:    "middle separator must fill the whole line",
			input:   "a\n==== \nb\n====\n",
			sep:     sep,
			wantErr: "found the closing \"====\" line but no \"====\" line between",
		},
		{
			name:    "old form without the closing separator",
			input:   "a\n====\nb\n",
			sep:     sep,
			wantErr: "closing \"====\" line is missing",
		},
		{
			name:    "old form deletion looks like a closing separator alone",
			input:   "a\n====\n",
			sep:     sep,
			wantErr: "no \"====\" line between the old and new blocks (an empty new block still takes two \"====\" lines",
		},
		{
			name:    "shell cut the heredoc inside the old block",
			input:   "a\nb\n",
			sep:     sep,
			wantErr: "closing \"====\" line is missing",
		},
		{
			name:    "shell cut the heredoc after a quoted separator in the old block",
			input:   "a\n====\nb\nc\n",
			sep:     sep,
			wantErr: "closing \"====\" line is missing",
		},
		{
			name:    "two separators before the closing one",
			input:   "a\n====\nb\n====\nc\n====\n",
			sep:     sep,
			wantErr: "found 2",
		},
		{
			name:    "closing separator alone",
			input:   "====\n",
			sep:     sep,
			wantErr: "found the closing \"====\" line but no \"====\" line between",
		},
		{
			name:    "empty old block",
			input:   "====\nb\n====\n",
			sep:     sep,
			wantErr: "old block is empty",
		},
		{
			name:    "identical blocks",
			input:   "same\n====\nsame\n====\n",
			sep:     sep,
			wantErr: "identical",
		},
		{
			name:    "empty input",
			input:   "",
			sep:     sep,
			wantErr: "closing \"====\" line is missing",
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
