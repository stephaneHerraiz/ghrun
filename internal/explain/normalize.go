// Package explain turns a failed GitHub Actions log into an explanation,
// backed by a self-enriching local RAG knowledge base and Claude.
package explain

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// contextBefore is how many lines of context are kept before each error line.
const contextBefore = 5

// maxRegionLines caps the extracted region so the embedded text stays focused.
const maxRegionLines = 400

// fallbackTailLines is used when no error marker matches at all.
const fallbackTailLines = 30

// errLineRe matches lines that look like errors, case-insensitively. The net
// is deliberately wide (fail/failed/failure/failing, fatal, panic, npm ERR!,
// exit code…): over-extracting beats missing the root cause. "exit code" also
// covers "Process completed with exit code N".
var errLineRe = regexp.MustCompile(`(?i)##\[error\]|\berror\b|\berr!|\bfail(ed|ure|ing)?\b|\bfatal\b|\bpanic\b|✗|exit code`)

// volatileRes maps volatile-token patterns to placeholders. Order matters:
// more specific patterns run first (timestamp before date/time, UUID before
// SHA, IP:port before bare line numbers).
var volatileRes = []struct {
	re  *regexp.Regexp
	rep string
}{
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`), "<TS>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}`), "<DATE>"},
	{regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}(\.\d+)?\b`), "<TIME>"},
	{regexp.MustCompile(`\b\d+(\.\d+)?(ms|s|m|h)\b`), "<DUR>"},
	{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<UUID>"},
	{regexp.MustCompile(`\b[0-9a-f]{7,40}\b`), "<SHA>"},
	{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?\b`), "<IP>"},
	// Directories are volatile, basenames are not: keep the filename so two
	// failures on different files don't collide on the same signature.
	{regexp.MustCompile(`/(?:home|tmp|__w|__t|var|usr|opt|Users|runner|github)[\w@./-]*/`), "<PATH>/"},
	// mktemp-style random leaves (tmp.XXXXXXXXXX, tmpXXXXXX) are volatile even
	// though they sit in basename position. Other random leaf names fall
	// through to the embedding-similarity path, which tolerates them.
	{regexp.MustCompile(`<PATH>/tmp\.?[A-Za-z0-9]{6,}\b`), "<PATH>/<TMP>"},
	{regexp.MustCompile(`:\d+(:\d+)?\b`), "<LN>"},
	{regexp.MustCompile(`\b\d{5,}\b`), "<NUM>"},
}

// extractErrorRegion keeps every line matching errLineRe plus contextBefore
// lines before each. With no match at all it falls back to the log tail, so
// there is always something to embed.
func extractErrorRegion(log string) string {
	lines := strings.Split(log, "\n")
	keep := make([]bool, len(lines))
	found := false
	for i, l := range lines {
		if errLineRe.MatchString(l) {
			found = true
			start := i - contextBefore
			if start < 0 {
				start = 0
			}
			for j := start; j <= i; j++ {
				keep[j] = true
			}
		}
	}
	if !found {
		start := len(lines) - fallbackTailLines
		if start < 0 {
			start = 0
		}
		return strings.Join(lines[start:], "\n")
	}
	var out []string
	for i, k := range keep {
		if k {
			out = append(out, lines[i])
		}
	}
	if len(out) > maxRegionLines {
		out = out[len(out)-maxRegionLines:]
	}
	return strings.Join(out, "\n")
}

// normalizeVolatile replaces volatile tokens (timestamps, paths, SHAs, IDs…)
// with placeholders so identical errors from different runs converge.
func normalizeVolatile(s string) string {
	for _, v := range volatileRes {
		s = v.re.ReplaceAllString(s, v.rep)
	}
	return s
}

// Prepare turns a raw failed log into the normalized text that is embedded
// and compared, plus its sha256 signature (exact-match fast path and chromem
// document ID).
func Prepare(log string) (normalized, signature string) {
	normalized = normalizeVolatile(extractErrorRegion(log))
	sum := sha256.Sum256([]byte(normalized))
	return normalized, hex.EncodeToString(sum[:])
}
