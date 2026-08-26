package backup

import (
	"context"
	"io"
	"os"
	"reflect"
	"testing"
)

type capturingLogWriter struct {
	lines []string
}

func (w *capturingLogWriter) WriteLine(message string) {
	w.lines = append(w.lines, message)
}

func TestLogLineWriterHandlesFragmentedWrites(t *testing.T) {
	log := &capturingLogWriter{}
	w := newLogLineWriter(log, "tool")

	assertWrite := func(chunk string) {
		t.Helper()
		n, err := w.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = (%d, %v), want (%d, nil)", chunk, n, err, len(chunk))
		}
	}
	assertWrite("fir")
	if len(log.lines) != 0 || string(w.pending) != "fir" {
		t.Fatalf("first fragment logged or buffered incorrectly: lines=%#v pending=%q", log.lines, w.pending)
	}
	assertWrite("st\nsec")
	if !reflect.DeepEqual(log.lines, []string{"[tool] first"}) || string(w.pending) != "sec" {
		t.Fatalf("second fragment handled incorrectly: lines=%#v pending=%q", log.lines, w.pending)
	}
	assertWrite("ond\n")
	if !reflect.DeepEqual(log.lines, []string{"[tool] first", "[tool] second"}) || len(w.pending) != 0 {
		t.Fatalf("completed fragments handled incorrectly: lines=%#v pending=%q", log.lines, w.pending)
	}
	if got := w.collected(); got != "first\nsecond" {
		t.Fatalf("collected() = %q, want complete raw output", got)
	}
}

func TestLogLineWriterEmitsMultipleCompleteLinesOnce(t *testing.T) {
	log := &capturingLogWriter{}
	w := newLogLineWriter(log, "tool")

	n, err := w.Write([]byte("one\ntwo\n\n  three  \r\n"))
	if err != nil || n != len("one\ntwo\n\n  three  \r\n") {
		t.Fatalf("Write() = (%d, %v)", n, err)
	}
	want := []string{"[tool] one", "[tool] two", "[tool] three"}
	if !reflect.DeepEqual(log.lines, want) {
		t.Fatalf("lines = %#v, want %#v", log.lines, want)
	}
	if len(w.pending) != 0 {
		t.Fatalf("complete input left pending bytes: %q", w.pending)
	}
}

func TestLogLineWriterFlushIsIdempotentAndPreservesCollection(t *testing.T) {
	log := &capturingLogWriter{}
	w := newLogLineWriter(log, "tool")
	_, _ = w.Write([]byte("complete\n  tail  "))

	w.Flush()
	w.Flush()
	want := []string{"[tool] complete", "[tool] tail"}
	if !reflect.DeepEqual(log.lines, want) {
		t.Fatalf("lines after repeated Flush = %#v, want %#v", log.lines, want)
	}
	if len(w.pending) != 0 {
		t.Fatalf("Flush left pending bytes: %q", w.pending)
	}
	if got := w.collected(); got != "complete\n  tail" {
		t.Fatalf("collected() = %q, want complete stderr independent of Flush", got)
	}
}

func TestPostgreSQLRunnerFlushesEachCommandTail(t *testing.T) {
	executor := &fakeCommandExecutor{runFunc: func(_ string, args []string, options CommandOptions) error {
		name := args[len(args)-1]
		_, _ = io.WriteString(options.Stdout, name)
		_, _ = io.WriteString(options.Stderr, "warning "+name)
		return nil
	}}
	log := &capturingLogWriter{}
	runner := NewPostgreSQLRunner(executor)
	result, err := runner.Run(context.Background(), TaskSpec{
		Name:    "pg-log-lines",
		TempDir: t.TempDir(),
		Database: DatabaseSpec{
			Host: "127.0.0.1", Port: 5432, User: "postgres", Names: []string{"app", "audit"},
		},
	}, log)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.TempDir) })

	for _, expected := range []string{"[pg_dump] warning app", "[pg_dump] warning audit"} {
		count := 0
		for _, line := range log.lines {
			if line == expected {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("line %q occurred %d times in %#v", expected, count, log.lines)
		}
	}
}

func TestMongoDBRunnerFlushesUnterminatedStderr(t *testing.T) {
	executor := &fakeCommandExecutor{runFunc: func(_ string, _ []string, options CommandOptions) error {
		_, _ = io.WriteString(options.Stdout, "archive")
		_, _ = io.WriteString(options.Stderr, "tail warning")
		return nil
	}}
	log := &capturingLogWriter{}
	runner := NewMongoDBRunner(executor)
	result, err := runner.Run(context.Background(), TaskSpec{
		Name: "mongo-log-tail",
		Database: DatabaseSpec{
			Host: "127.0.0.1", Port: 27017, Names: []string{"app"},
		},
	}, log)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.TempDir) })

	count := 0
	for _, line := range log.lines {
		if line == "[mongodump] tail warning" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("unterminated stderr line occurred %d times in %#v", count, log.lines)
	}
}
