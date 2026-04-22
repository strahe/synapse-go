package lifecycle

import (
	"errors"
	"testing"
)

func TestLifecycle_ClosedReturnsErrClosed(t *testing.T) {
	l := New()
	if err := l.CheckClosed(); err != nil {
		t.Fatalf("fresh lifecycle: CheckClosed returned %v, want nil", err)
	}
	l.Close()
	if err := l.CheckClosed(); !errors.Is(err, ErrClosed) {
		t.Fatalf("CheckClosed after Close: got %v, want ErrClosed", err)
	}
	if !l.IsClosed() {
		t.Fatal("IsClosed should be true after Close")
	}
	// Idempotent
	l.Close()
}

func TestLifecycle_NilReceiver(t *testing.T) {
	var l *Lifecycle
	if err := l.CheckClosed(); err != nil {
		t.Fatalf("nil: want nil, got %v", err)
	}
	if l.IsClosed() {
		t.Fatal("nil IsClosed should be false")
	}
	l.Close() // must not panic
}
