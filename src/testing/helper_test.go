// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	"regexp"
	"strings"
)

func TestTBHelper(t *T) {
	var buf bytes.Buffer
	ctx := newTestContext(1, newMatcher(regexp.MatchString, "", ""))
	t1 := &T{
		common: common{
			signal: make(chan bool),
			w:      &buf,
		},
		context: ctx,
	}
	t1.Run("Test", testHelper)

	want := `--- FAIL: Test (?s)
helperfuncs_test.go:12: 0
helperfuncs_test.go:33: 1
helperfuncs_test.go:21: 2
helperfuncs_test.go:35: 3
helperfuncs_test.go:42: 4
--- FAIL: Test/sub (?s)
helperfuncs_test.go:45: 5
helperfuncs_test.go:21: 6
helperfuncs_test.go:44: 7
helperfuncs_test.go:56: 8
helperfuncs_test.go:64: 9
helperfuncs_test.go:60: 10
`
	lines := strings.Split(buf.String(), "\n")
	durationRE := regexp.MustCompile(`\(.*\)$`)
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = durationRE.ReplaceAllString(line, "(?s)")
		lines[i] = line
	}
	got := strings.Join(lines, "\n")
	if got != want {
		t.Errorf("got output:\n\n%s\nwant:\n\n%s", got, want)
	}
}

func TestTBHelperParallel(t *T) {
	var buf bytes.Buffer
	ctx := newTestContext(1, newMatcher(regexp.MatchString, "", ""))
	t1 := &T{
		common: common{
			signal: make(chan bool),
			w:      &buf,
		},
		context: ctx,
	}
	t1.Run("Test", parallelTestHelper)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("parallelTestHelper gave %d lines of output; want 6", len(lines))
	}
	want := "helperfuncs_test.go:21: parallel"
	if got := strings.TrimSpace(lines[1]); got != want {
		t.Errorf("got output line %q; want %q", got, want)
	}
}

func TestTBHelperLineNumer(t *T) {
	var buf bytes.Buffer
	ctx := newTestContext(1, newMatcher(regexp.MatchString, "", ""))
	t1 := &T{
		common: common{
			signal: make(chan bool),
			w:      &buf,
		},
		context: ctx,
	}
	t1.Run("Test", func(t *T) {
		helperA := func(t *T) {
			t.Helper()
			t.Run("subtest", func(t *T) {
				t.Helper()
				t.Fatal("fatal error message")
			})
		}
		helperA(t)
	})

	want := "helper_test.go:92: fatal error message"
	got := ""
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) > 0 {
		got = strings.TrimSpace(lines[len(lines)-1])
	}
	if got != want {
		t.Errorf("got output:\n\n%v\nwant:\n\n%v", got, want)
	}
}

type noopWriter int

func (nw *noopWriter) Write(b []byte) (int, error) { return len(b), nil }

func BenchmarkTBHelper(b *B) {
	w := noopWriter(0)
	ctx := newTestContext(1, newMatcher(regexp.MatchString, "", ""))
	t1 := &T{
		common: common{
			signal: make(chan bool),
			w:      &w,
		},
		context: ctx,
	}
	f1 := func() {
		t1.Helper()
	}
	f2 := func() {
		t1.Helper()
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i&1 == 0 {
			f1()
		} else {
			f2()
		}
	}
}
