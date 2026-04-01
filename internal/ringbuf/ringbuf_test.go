package ringbuf

import (
	"bytes"
	"testing"
)

func TestNewPanicsOnZeroCapacity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for zero capacity")
		}
	}()
	New(0)
}

func TestWriteAndReadAll(t *testing.T) {
	r := New(10)

	r.Write([]byte("hello"))
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("hello")) {
		t.Errorf("expected %q, got %q", "hello", got)
	}

	// After ReadAll, buffer should be empty.
	got = r.ReadAll()
	if got != nil {
		t.Errorf("expected nil after second ReadAll, got %q", got)
	}
}

func TestMultipleWrites(t *testing.T) {
	r := New(10)

	r.Write([]byte("ab"))
	r.Write([]byte("cd"))
	r.Write([]byte("ef"))
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("abcdef")) {
		t.Errorf("expected %q, got %q", "abcdef", got)
	}
}

func TestOverflow(t *testing.T) {
	r := New(5)

	r.Write([]byte("abcde")) // fills exactly
	r.Write([]byte("fg"))    // overwrites "ab"
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("cdefg")) {
		t.Errorf("expected %q, got %q", "cdefg", got)
	}
}

func TestOverflowLargerThanCapacity(t *testing.T) {
	r := New(5)

	// Write more than capacity in one call.
	r.Write([]byte("abcdefgh"))
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("defgh")) {
		t.Errorf("expected %q, got %q", "defgh", got)
	}
}

func TestOverflowExactlyCapacity(t *testing.T) {
	r := New(5)

	r.Write([]byte("abcde"))
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("abcde")) {
		t.Errorf("expected %q, got %q", "abcde", got)
	}
}

func TestWriteAfterReadAll(t *testing.T) {
	r := New(5)

	r.Write([]byte("abc"))
	r.ReadAll()
	r.Write([]byte("xy"))
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("xy")) {
		t.Errorf("expected %q, got %q", "xy", got)
	}
}

func TestLen(t *testing.T) {
	r := New(5)

	if r.Len() != 0 {
		t.Errorf("expected len 0, got %d", r.Len())
	}

	r.Write([]byte("abc"))
	if r.Len() != 3 {
		t.Errorf("expected len 3, got %d", r.Len())
	}

	r.Write([]byte("defgh")) // overflow: cap is 5
	if r.Len() != 5 {
		t.Errorf("expected len 5 (capped), got %d", r.Len())
	}

	r.ReadAll()
	if r.Len() != 0 {
		t.Errorf("expected len 0 after ReadAll, got %d", r.Len())
	}
}

func TestEmptyWrite(t *testing.T) {
	r := New(5)
	r.Write(nil)
	r.Write([]byte{})
	if r.Len() != 0 {
		t.Errorf("expected len 0 after empty writes, got %d", r.Len())
	}
}

func TestWrapAroundMultipleTimes(t *testing.T) {
	r := New(4)

	// Fill and overflow several times.
	r.Write([]byte("aaaa"))
	r.Write([]byte("bb"))
	r.Write([]byte("ccc"))
	// Buffer should contain the last 4 bytes: "bccc"
	got := r.ReadAll()
	if !bytes.Equal(got, []byte("bccc")) {
		t.Errorf("expected %q, got %q", "bccc", got)
	}
}
