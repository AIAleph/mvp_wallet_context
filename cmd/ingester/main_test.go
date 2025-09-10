package main

import (
    "bytes"
    "flag"
    "os"
    "regexp"
    "strings"
    "testing"
    "time"
)

// exitPanic is used to intercept exit calls in tests.
type exitPanic struct{ code int }

func withFreshFlags(t *testing.T, fn func()) {
    t.Helper()
    old := flag.CommandLine
    // fresh flagset to avoid redefinition across multiple main() calls
    flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
    var buf bytes.Buffer
    flag.CommandLine.SetOutput(&buf)
    defer func() { flag.CommandLine = old }()
    fn()
}

func captureStd(t *testing.T, fn func()) (stdout, stderr string) {
    t.Helper()
    oldOut, oldErr := os.Stdout, os.Stderr
    rOut, wOut, _ := os.Pipe()
    rErr, wErr, _ := os.Pipe()
    os.Stdout, os.Stderr = wOut, wErr
    defer func() {
        os.Stdout, os.Stderr = oldOut, oldErr
    }()
    done := make(chan struct{})
    var outBuf, errBuf bytes.Buffer
    go func() { _, _ = outBuf.ReadFrom(rOut); close(done) }()
    go func() { _, _ = errBuf.ReadFrom(rErr) }()
    fn()
    _ = wOut.Close()
    _ = wErr.Close()
    <-done
    return outBuf.String(), errBuf.String()
}

func TestEnvHelpers(t *testing.T) {
    t.Setenv("FOO_BAR", "baz")
    if got := env("FOO_BAR", "zzz"); got != "baz" {
        t.Fatalf("env returned %q, want baz", got)
    }
    if got := env("NOPE", "def"); got != "def" {
        t.Fatalf("env default %q, want def", got)
    }

    t.Setenv("INT_OK", "42")
    if got := parseIntEnv("INT_OK", 7); got != 42 {
        t.Fatalf("parseIntEnv got %d, want 42", got)
    }
    t.Setenv("INT_BAD", "x")
    if got := parseIntEnv("INT_BAD", 7); got != 7 {
        t.Fatalf("parseIntEnv invalid got %d, want 7", got)
    }
    if got := parseIntEnv("INT_MISSING", 9); got != 9 {
        t.Fatalf("parseIntEnv missing got %d, want 9", got)
    }

    t.Setenv("DUR_OK", "150ms")
    if got := parseDurEnv("DUR_OK", time.Second); got != 150*time.Millisecond {
        t.Fatalf("parseDurEnv got %v, want 150ms", got)
    }
    t.Setenv("DUR_BAD", "nope")
    if got := parseDurEnv("DUR_BAD", time.Second); got != time.Second {
        t.Fatalf("parseDurEnv invalid got %v, want 1s", got)
    }
}

func TestPrintUsage(t *testing.T) {
    withFreshFlags(t, func() {
        var buf bytes.Buffer
        oldOut := flag.CommandLine.Output()
        flag.CommandLine.SetOutput(&buf)
        defer flag.CommandLine.SetOutput(oldOut)
        printUsage()
        s := buf.String()
        if !strings.Contains(s, "Usage:") || !strings.Contains(s, "Environment variables") {
            t.Fatalf("unexpected usage output: %q", s)
        }
    })
}

func TestMain_ShowVersion(t *testing.T) {
    withFreshFlags(t, func() {
        version = "test-version"
        oldArgs := os.Args
        os.Args = []string{"ingester", "-version"}
        defer func() { os.Args = oldArgs }()
        out, _ := captureStd(t, func() { main() })
        if strings.TrimSpace(out) != "test-version" {
            t.Fatalf("version output = %q", out)
        }
    })
}

func TestMain_MissingAddress(t *testing.T) {
    withFreshFlags(t, func() {
        oldArgs := os.Args
        os.Args = []string{"ingester"}
        defer func() { os.Args = oldArgs }()
        oldExit := exit
        defer func() { exit = oldExit }()
        exit = func(code int) { panic(exitPanic{code}) }
        _, errOut := captureStd(t, func() {
            defer func() {
                if r := recover(); r != nil {
                    if ep, ok := r.(exitPanic); ok {
                        if ep.code != 2 { t.Fatalf("exit code %d, want 2", ep.code) }
                        return
                    }
                    panic(r)
                }
                t.Fatalf("expected exit panic")
            }()
            main()
        })
        if !strings.Contains(errOut, "missing --address") {
            t.Fatalf("stderr = %q", errOut)
        }
    })
}

func TestMain_InvalidAddress(t *testing.T) {
    withFreshFlags(t, func() {
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", "nothex"}
        defer func() { os.Args = oldArgs }()
        oldExit := exit
        defer func() { exit = oldExit }()
        exit = func(code int) { panic(exitPanic{code}) }
        _, errOut := captureStd(t, func() {
            defer func() {
                if r := recover(); r != nil {
                    if ep, ok := r.(exitPanic); ok && ep.code == 2 { return }
                    panic(r)
                }
                t.Fatalf("expected exit panic")
            }()
            main()
        })
        if !strings.Contains(errOut, "invalid --address") {
            t.Fatalf("stderr = %q", errOut)
        }
    })
}

func TestMain_UnknownMode(t *testing.T) {
    withFreshFlags(t, func() {
        oldArgs := os.Args
        addr := "0x" + strings.Repeat("a", 40)
        os.Args = []string{"ingester", "--address", addr, "--mode", "weird"}
        defer func() { os.Args = oldArgs }()
        oldExit := exit
        defer func() { exit = oldExit }()
        exit = func(code int) { panic(exitPanic{code}) }
        _, errOut := captureStd(t, func() {
            defer func() {
                if r := recover(); r != nil {
                    if ep, ok := r.(exitPanic); ok && ep.code == 2 { return }
                    panic(r)
                }
                t.Fatalf("expected exit panic")
            }()
            main()
        })
        if !strings.Contains(errOut, "unknown --mode") {
            t.Fatalf("stderr = %q", errOut)
        }
    })
}

func TestMain_FromGreaterThanTo(t *testing.T) {
    withFreshFlags(t, func() {
        addr := "0x" + strings.Repeat("a", 40)
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", addr, "--from-block", "10", "--to-block", "5"}
        defer func() { os.Args = oldArgs }()
        oldExit := exit
        defer func() { exit = oldExit }()
        exit = func(code int) { panic(exitPanic{code}) }
        _, errOut := captureStd(t, func() {
            defer func() {
                if r := recover(); r != nil {
                    if ep, ok := r.(exitPanic); ok && ep.code == 2 { return }
                    panic(r)
                }
                t.Fatalf("expected exit panic")
            }()
            main()
        })
        if !strings.Contains(errOut, "from-block cannot be greater") {
            t.Fatalf("stderr = %q", errOut)
        }
    })
}

func TestMain_NegativeConfirmations(t *testing.T) {
    withFreshFlags(t, func() {
        addr := "0x" + strings.Repeat("a", 40)
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", addr, "--confirmations", "-1"}
        defer func() { os.Args = oldArgs }()
        oldExit := exit
        defer func() { exit = oldExit }()
        exit = func(code int) { panic(exitPanic{code}) }
        _, errOut := captureStd(t, func() {
            defer func() {
                if r := recover(); r != nil {
                    if ep, ok := r.(exitPanic); ok && ep.code == 2 { return }
                    panic(r)
                }
                t.Fatalf("expected exit panic")
            }()
            main()
        })
        if !strings.Contains(errOut, "must be >= 0") {
            t.Fatalf("stderr = %q", errOut)
        }
    })
}

func TestMain_NonPositiveBatch(t *testing.T) {
    withFreshFlags(t, func() {
        addr := "0x" + strings.Repeat("a", 40)
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", addr, "--batch", "0"}
        defer func() { os.Args = oldArgs }()
        oldExit := exit
        defer func() { exit = oldExit }()
        exit = func(code int) { panic(exitPanic{code}) }
        _, errOut := captureStd(t, func() {
            defer func() {
                if r := recover(); r != nil {
                    if ep, ok := r.(exitPanic); ok && ep.code == 2 { return }
                    panic(r)
                }
                t.Fatalf("expected exit panic")
            }()
            main()
        })
        if !strings.Contains(errOut, "must be > 0") {
            t.Fatalf("stderr = %q", errOut)
        }
    })
}

func TestMain_DryRunOutputsJSON(t *testing.T) {
    withFreshFlags(t, func() {
        addr := "0x" + strings.Repeat("a", 40)
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", addr, "--dry-run", "--mode", "backfill", "--from-block", "1", "--to-block", "2", "--batch", "10", "--timeout", "100ms"}
        defer func() { os.Args = oldArgs }()
        out, _ := captureStd(t, func() { main() })
        if !strings.Contains(out, "\"address\"") || !strings.Contains(out, "\"mode\"") {
            t.Fatalf("unexpected dry-run output: %q", out)
        }
        // sanity: contains block numbers
        re := regexp.MustCompile(`"from_block":\s*1`)
        if !re.MatchString(out) {
            t.Fatalf("from_block missing in output: %q", out)
        }
    })
}

func TestMain_BackfillOK(t *testing.T) {
    withFreshFlags(t, func() {
        addr := "0x" + strings.Repeat("a", 40)
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", addr}
        defer func() { os.Args = oldArgs }()
        out, _ := captureStd(t, func() { main() })
        if strings.TrimSpace(out) != "ok" {
            t.Fatalf("stdout = %q, want ok", out)
        }
    })
}

func TestMain_DeltaOK(t *testing.T) {
    withFreshFlags(t, func() {
        addr := "0x" + strings.Repeat("a", 40)
        oldArgs := os.Args
        os.Args = []string{"ingester", "--address", addr, "--mode", "delta"}
        defer func() { os.Args = oldArgs }()
        out, _ := captureStd(t, func() { main() })
        if strings.TrimSpace(out) != "ok" {
            t.Fatalf("stdout = %q, want ok", out)
        }
    })
}
