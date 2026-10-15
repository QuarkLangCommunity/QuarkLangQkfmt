package main

// Correctness tests for the two promises the formatter makes in README.md:
//
//  1. only whitespace changes and the output is idempotent
//     ("只改空白，不增删任何 token"; "输出幂等（两遍格式化结果字节一致）")
//  2. `-l` lists exactly the files whose bytes would change and exits 1 when it listed any
//     ("只列出未格式化的文件（CI 用；有差异退出码 1）")
//
// bench_test.go next to this file measures speed only; nothing here asserts anything about timing.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// canonical is already formatted, unformatted differs from it only in whitespace.
const canonical = "program main;\n\nfn main(IOStream io) {\n    io.println(\"ok\");\n}\n"

var formatCases = []struct{ name, src string }{
	{"empty", ""},
	{"blank-only", "\n\n\n"},
	{"already-canonical", canonical},
	{"missing-indent", "program main;\nfn main(IOStream io) {\nio.println(\"ok\");\n}\n"},
	{"operator-spacing", "program main;\nfn main(IOStream io) {\n    int a=1+2*3;\n}\n"},
	{"unary-operator", "program main;\nfn main(IOStream io) {\n    int a = -1;\n    bool b = !false;\n}\n"},
	{"trailing-line-comment", "program main;\nfn main(IOStream io) {\n    int a = 1;// note\n}\n"},
	{"block-comment", "program main;\n/* a\n   b */\nfn main(IOStream io) {\n}\n"},
	{"blank-line-collapse", "program main;\n\n\n\nfn main(IOStream io) {\n}\n"},
	{"else-on-own-line", "program main;\nfn main(IOStream io) {\n    if (1 > 0) {\n        io.println(\"a\");\n    }\n    else {\n        io.println(\"b\");\n    }\n}\n"},
	{"trailing-whitespace", "program main;   \nfn main(IOStream io) {\n}\n"},
	{"space-block", "space {\nfn f(int a) int {\nreturn a;\n}\n} s;\n"},
	{"no-final-newline", "program main;\nfn main(IOStream io) {\n}"},
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, tc := range formatCases {
		t.Run(tc.name, func(t *testing.T) {
			once, err := format(tc.src)
			if err != nil {
				t.Fatalf("format returned an error: %v", err)
			}
			twice, err := format(once)
			if err != nil {
				t.Fatalf("format(format(src)) returned an error: %v", err)
			}
			if once != twice {
				t.Errorf("formatting is not idempotent\nfirst pass:  %q\nsecond pass: %q", once, twice)
			}
		})
	}
}

func TestFormatChangesWhitespaceOnly(t *testing.T) {
	for _, tc := range formatCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := format(tc.src)
			if err != nil {
				t.Fatalf("format returned an error: %v", err)
			}
			before, err := scan(tc.src)
			if err != nil {
				t.Fatalf("scan(src) returned an error: %v", err)
			}
			after, err := scan(out)
			if err != nil {
				t.Fatalf("scan(format(src)) returned an error: %v", err)
			}
			if len(before) != len(after) {
				t.Fatalf("token count changed: %d -> %d", len(before), len(after))
			}
			for i := range before {
				if before[i].k != after[i].k || before[i].text != after[i].text {
					t.Fatalf("token %d changed: kind %v %q -> kind %v %q",
						i, before[i].k, before[i].text, after[i].k, after[i].text)
				}
			}
		})
	}
}

// TestListReportsExactlyTheUnformattedFiles pins the CI contract of -l: the files it prints are
// exactly the files whose bytes change, and the exit code is 1 only when it printed something.
func TestListReportsExactlyTheUnformattedFiles(t *testing.T) {
	exe := buildQkfmt(t)
	dir := t.TempDir()
	formatted := filepath.Join(dir, "formatted.qk")
	unformatted := filepath.Join(dir, "unformatted.qk")
	writeFile(t, formatted, canonical)
	writeFile(t, unformatted, "program main;\nfn main(IOStream io) {\nio.println(\"ok\");\n}\n")

	out, code := runQkfmt(t, exe, "-l", formatted, unformatted)
	if code != 1 {
		t.Errorf("-l exited %d, want 1 (one file is unformatted)", code)
	}
	if got, want := strings.TrimRight(out, "\r\n"), unformatted; got != want {
		t.Errorf("-l printed %q, want exactly %q", got, want)
	}

	out, code = runQkfmt(t, exe, "-l", formatted)
	if code != 0 {
		t.Errorf("-l exited %d on a formatted file, want 0", code)
	}
	if out != "" {
		t.Errorf("-l printed %q on a formatted file, want nothing", out)
	}
}

// TestWriteMakesListClean covers the documented CI recipe: after -w, -l must be silent and exit 0,
// and the bytes -w leaves behind must be exactly what the formatter computes for that input.
func TestWriteMakesListClean(t *testing.T) {
	exe := buildQkfmt(t)
	path := filepath.Join(t.TempDir(), "sample.qk")
	src := "program main;\nfn main(IOStream io) {\nio.println(\"ok\");\n}\n"
	want, err := format(src)
	if err != nil {
		t.Fatal(err)
	}
	if want == src {
		t.Fatal("fixture is already formatted, so this test would prove nothing")
	}
	writeFile(t, path, src)

	if _, code := runQkfmt(t, exe, "-l", path); code != 1 {
		t.Fatalf("-l exited %d before -w, want 1", code)
	}
	if _, code := runQkfmt(t, exe, "-w", path); code != 0 {
		t.Fatalf("-w exited %d, want 0", code)
	}
	out, code := runQkfmt(t, exe, "-l", path)
	if code != 0 || out != "" {
		t.Fatalf("after -w: -l exited %d and printed %q, want 0 and nothing", code, out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("after -w the file is %q, want %q", got, want)
	}
}

// TestCanonicalSampleIsAFixedPoint guards the fixture the -l test relies on: if the formatter ever
// starts rewriting this sample, the "already formatted" half of that test would silently stop
// testing anything.
func TestCanonicalSampleIsAFixedPoint(t *testing.T) {
	out, err := format(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if out != canonical {
		t.Errorf("canonical sample is not a fixed point: format gave %q, want %q", out, canonical)
	}
}

// TestFormatAppliesDocumentedWhitespaceRules pins the exact output for the whitespace rules the
// README promises, so a regression in any single rule fails by name rather than only by luck of
// the idempotence check above.
func TestFormatAppliesDocumentedWhitespaceRules(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"four-space-indent",
			"program main;\nfn main(IOStream io) {\nio.println(\"ok\");\n}\n",
			"program main;\nfn main(IOStream io) {\n    io.println(\"ok\");\n}\n",
		},
		{
			"trailing-whitespace-removed",
			"program main;   \nfn main(IOStream io) {\n}\n",
			"program main;\nfn main(IOStream io) {\n}\n",
		},
		{
			"blank-runs-collapse-to-one",
			"program main;\n\n\n\nfn main(IOStream io) {\n}\n",
			"program main;\n\nfn main(IOStream io) {\n}\n",
		},
		{
			"exactly-one-final-newline",
			"program main;\nfn main(IOStream io) {\n}",
			"program main;\nfn main(IOStream io) {\n}\n",
		},
		{"empty-input-still-one-newline", "", "\n"},
		{"blank-only-input-still-one-newline", "\n\n\n", "\n"},
		{
			"binary-operators-spaced",
			"program main;\nfn main(IOStream io) {\n    int a=1+2*3;\n}\n",
			"program main;\nfn main(IOStream io) {\n    int a = 1 + 2 * 3;\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := format(tc.in)
			if err != nil {
				t.Fatalf("format returned an error: %v", err)
			}
			if got != tc.want {
				t.Errorf("format(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildQkfmt(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "qkfmt")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

func runQkfmt(t *testing.T, exe string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(exe, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	if stderr.Len() > 0 {
		t.Logf("stderr from %v: %s", args, stderr.String())
	}
	return stdout.String(), code
}
