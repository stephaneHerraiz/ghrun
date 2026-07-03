package explain

import (
	"fmt"
	"strings"
	"testing"
)

func TestExtractErrorRegionMatchesVariants(t *testing.T) {
	cases := []struct{ name, line string }{
		{"gh error marker", "##[error]Process failed"},
		{"plain error", "Error: something broke"},
		{"npm err", "npm ERR! code ELIFECYCLE"},
		{"fail lower", "1 test fail"},
		{"failed upper", "2 tests FAILED"},
		{"failure mixed", "Failure while linking"},
		{"failing", "3 failing"},
		{"fatal", "fatal: repository not found"},
		{"panic", "panic: runtime error: index out of range"},
		{"cross mark", "✗ should compile"},
		{"exit code", "Process completed with exit code 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := "line A\nline B\n" + c.line + "\ntrailing ok"
			got := extractErrorRegion(log)
			if !strings.Contains(got, c.line) {
				t.Fatalf("region %q does not contain %q", got, c.line)
			}
		})
	}
}

func TestExtractErrorRegionSkipsCleanLines(t *testing.T) {
	log := "downloading dependencies\ncompiling module alpha\nError: boom"
	got := extractErrorRegion(log)
	if !strings.Contains(got, "Error: boom") {
		t.Fatalf("region %q misses the error line", got)
	}
	// Clean lines far from any error line must not survive.
	long := strings.Repeat("clean build output\n", 20) + "unrelated middle\n" + strings.Repeat("more clean output\n", 20) + "Error: boom"
	got = extractErrorRegion(long)
	if strings.Contains(got, "unrelated middle") {
		t.Fatalf("region %q kept a line outside the context window", got)
	}
}

func TestExtractErrorRegionKeepsContextBefore(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "setup line %d\n", i)
	}
	b.WriteString("Error: boom")
	got := extractErrorRegion(b.String())
	if !strings.Contains(got, "setup line 6") {
		t.Errorf("region %q misses context line 6 (want 5 lines before)", got)
	}
	if strings.Contains(got, "setup line 5\n") {
		t.Errorf("region %q kept more than 5 context lines", got)
	}
}

func TestExtractErrorRegionFallsBackToTail(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, "ok step %d\n", i)
	}
	got := extractErrorRegion(b.String())
	if !strings.Contains(got, "ok step 100") {
		t.Errorf("fallback region %q misses the log tail", got)
	}
	if strings.Contains(got, "ok step 1\n") {
		t.Errorf("fallback region %q kept the whole log", got)
	}
}

func TestNormalizeVolatileClasses(t *testing.T) {
	cases := []struct{ name, in, wantGone, wantPlaceholder string }{
		{"timestamp", "at 2026-07-03T10:15:42.123Z step", "2026-07-03T10:15:42.123Z", "<TS>"},
		{"uuid", "request 3f2b8a10-9c4d-4e2a-8b1a-77aa00bb11cc failed", "3f2b8a10-9c4d-4e2a-8b1a-77aa00bb11cc", "<UUID>"},
		{"sha", "commit a1b2c3d4e5f failed", "a1b2c3d4e5f", "<SHA>"},
		{"ip port", "dial 192.168.1.10:8080 refused", "192.168.1.10:8080", "<IP>"},
		{"abs path", "open /home/runner/work/app/main.go failed", "/home/runner/work/app", "<PATH>"},
		{"tmp path", "wrote /tmp/build-artifacts/out.bin then failed", "/tmp/build-artifacts", "<PATH>"},
		{"duration", "test timed out after 300s of failure", "300s", "<DUR>"},
		{"line number", "main.go:127:3: undefined: foo error", ":127:3", "<LN>"},
		// 5 digits: below the SHA pattern's 7-char floor, so <NUM> catches it.
		// Longer all-digit runs are hex-valid and become <SHA> — mislabeled but
		// stable, which is all the signature needs.
		{"long id", "run 12345 failed", "12345", "<NUM>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeVolatile(c.in)
			if strings.Contains(got, c.wantGone) {
				t.Errorf("normalizeVolatile(%q) = %q — still contains %q", c.in, got, c.wantGone)
			}
			if !strings.Contains(got, c.wantPlaceholder) {
				t.Errorf("normalizeVolatile(%q) = %q — missing %q", c.in, got, c.wantPlaceholder)
			}
		})
	}
}

func TestPrepareSignatureStable(t *testing.T) {
	logA := "build\t2026-07-03T10:00:01Z compiling /home/runner/work/a.go\nbuild\t2026-07-03T10:00:02Z Error: undefined symbol (run 111222333)"
	logB := "build\t2026-07-04T22:59:59Z compiling /home/other/dir/a.go\nbuild\t2026-07-04T23:00:00Z Error: undefined symbol (run 999888777)"
	_, sigA := Prepare(logA)
	_, sigB := Prepare(logB)
	if sigA != sigB {
		t.Errorf("signatures differ for volatile-only changes:\n%s\n%s", sigA, sigB)
	}
	_, sigC := Prepare("build\tError: a completely different failure")
	if sigC == sigA {
		t.Errorf("different errors share signature %s", sigC)
	}
	if len(sigA) != 64 {
		t.Errorf("signature %q is not a sha256 hex string", sigA)
	}
}

func TestPrepareSignatureDistinguishesFilenames(t *testing.T) {
	logA := "build\tError: /home/runner/work/repo/config/prod.yaml: invalid syntax"
	logB := "build\tError: /home/runner/work/repo/config/staging.yaml: invalid syntax"
	_, sigA := Prepare(logA)
	_, sigB := Prepare(logB)
	if sigA == sigB {
		t.Error("different basenames must not share a signature")
	}
	logC := "build\tError: /home/elsewhere/other/dir/prod.yaml: invalid syntax"
	_, sigC := Prepare(logC)
	if sigC != sigA {
		t.Error("same basename in a different directory must share the signature")
	}
}
