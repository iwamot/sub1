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
			input:   "foo\n====\nbar\n",
			sep:     sep,
			wantOld: "foo",
			wantNew: "bar",
		},
		{
			name:    "multi-line blocks keep inner newlines and indentation",
			input:   "  if x:\n    return\n====\n  if x:\n    return None\n",
			sep:     sep,
			wantOld: "  if x:\n    return",
			wantNew: "  if x:\n    return None",
		},
		{
			name:    "empty new block deletes",
			input:   "gone\n====\n",
			sep:     sep,
			wantOld: "gone",
			wantNew: "",
		},
		{
			name:    "no trailing newline after new block",
			input:   "a\n====\nb",
			sep:     sep,
			wantOld: "a",
			wantNew: "b",
		},
		{
			name:    "blank line inside old block is kept",
			input:   "a\n\nb\n====\nc\n",
			sep:     sep,
			wantOld: "a\n\nb",
			wantNew: "c",
		},
		{
			name:    "custom separator",
			input:   "Title\n====\n%%\nTitle\n----\n",
			sep:     []byte("%%"),
			wantOld: "Title\n====",
			wantNew: "Title\n----",
		},
		{
			name:    "separator must fill the whole line",
			input:   "a\n==== \nb\n",
			sep:     sep,
			wantErr: "found 0",
		},
		{
			name:    "no separator",
			input:   "a\nb\n",
			sep:     sep,
			wantErr: "found 0",
		},
		{
			name:    "two separators",
			input:   "a\n====\nb\n====\nc\n",
			sep:     sep,
			wantErr: "found 2",
		},
		{
			name:    "empty old block",
			input:   "====\nb\n",
			sep:     sep,
			wantErr: "old block is empty",
		},
		{
			name:    "identical blocks",
			input:   "same\n====\nsame\n",
			sep:     sep,
			wantErr: "identical",
		},
		{
			name:    "empty input",
			input:   "",
			sep:     sep,
			wantErr: "found 0",
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
