package occur

import (
	"bytes"
	"math/rand"
	"slices"
	"testing"
	"testing/quick"
)

// Generated inputs run in ordinary go test, so CI checks the properties as
// well as the explicit examples. A fixed seed makes failures reproducible.
func propertyConfig() *quick.Config {
	return &quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(1))}
}

func TestLinesReportsGeneratedLineNumbers(t *testing.T) {
	checked := 0
	property := func(before, old, between, after []byte, copies uint8) bool {
		if len(old) == 0 {
			return true
		}
		n := 1 + int(copies%16)
		chunks := [][]byte{before}
		for i := range n {
			if i > 0 {
				chunks = append(chunks, between)
			}
			chunks = append(chunks, old)
		}
		chunks = append(chunks, after)
		content := bytes.Join(chunks, nil)
		// Keep inputs where the standard library selects exactly the
		// inserted blocks, including adjacent and multiline occurrences.
		parts := bytes.Split(content, old)
		if len(parts) != n+1 || !bytes.Equal(parts[0], before) || !bytes.Equal(parts[n], after) {
			return true
		}
		for _, part := range parts[1:n] {
			if !bytes.Equal(part, between) {
				return true
			}
		}
		checked++
		first := 1 + bytes.Count(before, newline)
		step := bytes.Count(old, newline) + bytes.Count(between, newline)
		want := make([]int, n)
		for i := range want {
			want[i] = first + i*step
		}
		savedContent, savedOld := bytes.Clone(content), bytes.Clone(old)
		return slices.Equal(Lines(content, old), want) &&
			bytes.Equal(content, savedContent) && bytes.Equal(old, savedOld)
	}
	if err := quick.Check(property, propertyConfig()); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no generated input selected the inserted blocks")
	}
	t.Logf("checked %d generated match layouts", checked)
}

func TestReplacePreservesSurroundings(t *testing.T) {
	checked := 0
	property := func(before, old, after, replacement []byte) bool {
		if len(old) == 0 || len(replacement) == 0 {
			return true // empty old is invalid; deletion has separate properties
		}
		content := bytes.Join([][]byte{before, old, after}, nil)
		if bytes.Index(content, old) != len(before) || bytes.Count(content, old) != 1 {
			return true // this property concerns a match at the chosen boundary
		}
		checked++
		want := bytes.Join([][]byte{before, replacement, after}, nil)
		savedContent, savedOld, savedNew := bytes.Clone(content), bytes.Clone(old), bytes.Clone(replacement)
		got := Replace(content, old, replacement)
		return bytes.Equal(got, want) && bytes.Equal(content, savedContent) &&
			bytes.Equal(old, savedOld) && bytes.Equal(replacement, savedNew)
	}
	if err := quick.Check(property, propertyConfig()); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no generated input had a match at the chosen boundary")
	}
	t.Logf("checked %d unique replacements", checked)
}

func TestDeleteInlinePreservesSurroundings(t *testing.T) {
	checked := 0
	property := func(before, old, after []byte) bool {
		if len(old) == 0 {
			return true
		}
		// The preceding byte makes this an inline occurrence regardless of
		// the generated block's contents or the following line ending.
		before = append(bytes.Clone(before), '>')
		content := bytes.Join([][]byte{before, old, after}, nil)
		if bytes.Index(content, old) != len(before) || bytes.Count(content, old) != 1 {
			return true
		}
		checked++
		want := bytes.Join([][]byte{before, after}, nil)
		return deletionPreservesInputs(content, old, want)
	}
	if err := quick.Check(property, propertyConfig()); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no generated input had an inline match at the chosen boundary")
	}
	t.Logf("checked %d inline deletions", checked)
}

func TestDeleteWholeBlockPreservesSurroundings(t *testing.T) {
	checked := 0
	property := func(before, old, after []byte, crlf, first, terminal bool) bool {
		if len(old) == 0 || bytes.HasSuffix(old, newline) {
			return true // a block already ending in LF carries its own break
		}
		ending := []byte("\n")
		if crlf {
			ending = []byte("\r\n")
		}
		if first {
			before = nil
		} else {
			before = bytes.Join([][]byte{before, ending}, nil)
		}
		if terminal {
			ending, after = nil, nil
		}
		content := bytes.Join([][]byte{before, old, ending, after}, nil)
		if bytes.Index(content, old) != len(before) || bytes.Count(content, old) != 1 {
			return true
		}
		checked++
		want := bytes.Join([][]byte{before, after}, nil)
		return deletionPreservesInputs(content, old, want)
	}
	if err := quick.Check(property, propertyConfig()); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no generated input had a whole-block match at the chosen boundary")
	}
	t.Logf("checked %d whole-block deletions", checked)
}

func deletionPreservesInputs(content, old, want []byte) bool {
	savedContent, savedOld := bytes.Clone(content), bytes.Clone(old)
	got := Replace(content, old, nil)
	return bytes.Equal(got, want) && bytes.Equal(content, savedContent) && bytes.Equal(old, savedOld)
}
