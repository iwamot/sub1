// Package crlf tells CRLF files apart from LF files and converts between the
// two line endings.
//
// sub1's blocks come from a heredoc, which the shell ends with LF, so a CRLF
// file could never be matched or edited as literal bytes: a one-line old block
// would match and the new block would then be written with LF, leaving the
// file with mixed endings. When every line of the file ends with CRLF, the
// blocks are read as CRLF too.
package crlf

import "bytes"

var (
	lf   = []byte("\n")
	crlf = []byte("\r\n")
)

// Uniform reports whether content has at least one line break and every one
// of them is CRLF. A file with no line break is not CRLF: there is nothing
// to say what its line ending is, and it is left as LF.
func Uniform(content []byte) bool {
	n := bytes.Count(content, lf)
	return n > 0 && bytes.Count(content, crlf) == n
}

// ToCRLF turns every LF in b into CRLF. b must not contain CR already.
func ToCRLF(b []byte) []byte {
	return bytes.ReplaceAll(b, lf, crlf)
}

// ToLF turns every CRLF in b into LF, leaving any other CR alone.
func ToLF(b []byte) []byte {
	return bytes.ReplaceAll(b, crlf, lf)
}
