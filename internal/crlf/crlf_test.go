package crlf

import (
	"bytes"
	"testing"
)

// samples are every string of up to six bytes over "a", CR, and LF, so that
// lone CRs, CRLFs, CRs before CRLFs and missing final line breaks all occur,
// and the longer strings the examples used to name.
func samples() [][]byte {
	out := [][]byte{{}}
	frontier := [][]byte{{}}
	for range 6 {
		var next [][]byte
		for _, s := range frontier {
			for _, c := range []byte("a\r\n") {
				next = append(next, append(bytes.Clone(s), c))
			}
		}
		out = append(out, next...)
		frontier = next
	}
	for _, s := range []string{"abc", "a\r\nb\nc\r\n", "a\rb\r\n", "a\r\nb\r\n", "\n\n"} {
		out = append(out, []byte(s))
	}
	return out
}

// lineEndings walks b byte by byte and counts its line breaks and how many of
// them have a CR right before them. It is the definition of the line endings
// written out, kept apart from the byte searches the package uses.
func lineEndings(b []byte) (breaks, crlfs int) {
	for i, c := range b {
		if c == '\n' {
			breaks++
			if i > 0 && b[i-1] == '\r' {
				crlfs++
			}
		}
	}
	return breaks, crlfs
}

// Uniform is true exactly when there is a line break and every one has a CR
// before it.
func TestUniformMatchesTheLineEndings(t *testing.T) {
	for _, s := range samples() {
		saved := bytes.Clone(s)
		breaks, crlfs := lineEndings(s)
		if got, want := Uniform(s), breaks > 0 && crlfs == breaks; got != want {
			t.Fatalf("Uniform(%q) = %v, want %v", s, got, want)
		}
		if !bytes.Equal(s, saved) {
			t.Fatalf("Uniform changed %q", saved)
		}
	}
}

// For text without CR, which is what a block from a heredoc is, ToCRLF puts
// a CR before every LF and nothing else, the result is uniform whenever the
// text has a line break, and ToLF takes it back to the text.
func TestToCRLFAndBackForTextWithoutCR(t *testing.T) {
	checked := 0
	for _, s := range samples() {
		if bytes.ContainsRune(s, '\r') {
			continue
		}
		checked++
		saved := bytes.Clone(s)
		got := ToCRLF(s)
		breaks, _ := lineEndings(s)
		gotBreaks, gotCRLFs := lineEndings(got)
		if gotBreaks != breaks || gotCRLFs != breaks || len(got) != len(s)+breaks {
			t.Fatalf("ToCRLF(%q) = %q", s, got)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r"), nil), s) {
			t.Fatalf("ToCRLF(%q) = %q adds more than CRs", s, got)
		}
		if Uniform(got) != (breaks > 0) {
			t.Fatalf("Uniform(ToCRLF(%q)) = %v", s, Uniform(got))
		}
		if back := ToLF(got); !bytes.Equal(back, s) {
			t.Fatalf("ToLF(ToCRLF(%q)) = %q", s, back)
		}
		if !bytes.Equal(s, saved) {
			t.Fatalf("ToCRLF changed %q", saved)
		}
	}
	if checked == 0 {
		t.Fatal("no sample without CR")
	}
}

// ToLF drops the CR of each CRLF and leaves every other byte, a CR that is
// not right before an LF included.
func TestToLFDropsOnlyTheCROfEachCRLF(t *testing.T) {
	for _, s := range samples() {
		saved := bytes.Clone(s)
		var want []byte
		for i, c := range s {
			if c == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				continue
			}
			want = append(want, c)
		}
		if got := ToLF(s); !bytes.Equal(got, want) {
			t.Fatalf("ToLF(%q) = %q, want %q", s, got, want)
		}
		if !bytes.Equal(s, saved) {
			t.Fatalf("ToLF changed %q", saved)
		}
	}
}
