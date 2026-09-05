package crlf

import "testing"

func TestUniform(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"no line break", "abc", false},
		{"lone CR is not a line break", "a\rb", false},
		{"LF", "a\nb\n", false},
		{"CRLF", "a\r\nb\r\n", true},
		{"CRLF without final line break", "a\r\nb", true},
		{"mixed", "a\r\nb\nc\r\n", false},
		{"CRLF with a stray CR inside a line", "a\rb\r\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Uniform([]byte(tt.content)); got != tt.want {
				t.Errorf("Uniform(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestToCRLF(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"a\nb", "a\r\nb"},
		{"a\nb\n", "a\r\nb\r\n"},
		{"\n\n", "\r\n\r\n"},
	}
	for _, tt := range tests {
		if got := string(ToCRLF([]byte(tt.in))); got != tt.want {
			t.Errorf("ToCRLF(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToLF(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a\r\nb\r\n", "a\nb\n"},
		{"a\nb\n", "a\nb\n"},
		{"a\rb\r\n", "a\rb\n"},
	}
	for _, tt := range tests {
		if got := string(ToLF([]byte(tt.in))); got != tt.want {
			t.Errorf("ToLF(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
