package ringbuffer

import "testing"

func TestNewDefaultsToFallbackCapacity(t *testing.T) {
	buf := New(0)
	if buf.capacity != 4096 {
		t.Fatalf("expected fallback capacity 4096, got %d", buf.capacity)
	}
}

func TestWriteDropsOldestBytesOnOverflow(t *testing.T) {
	buf := New(5)
	if _, err := buf.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := buf.Write([]byte("def")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := string(buf.Bytes()); got != "bcdef" {
		t.Fatalf("expected tail to be kept, got %q", got)
	}
}

func TestWriteLargeChunkKeepsOnlyTail(t *testing.T) {
	buf := New(4)
	if _, err := buf.Write([]byte("abcdef")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := string(buf.Bytes()); got != "cdef" {
		t.Fatalf("expected last 4 bytes, got %q", got)
	}
}

func TestBytesReturnsCopy(t *testing.T) {
	buf := New(8)
	if _, err := buf.Write([]byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	snapshot := buf.Bytes()
	snapshot[0] = 'j'

	if got := string(buf.Bytes()); got != "hello" {
		t.Fatalf("expected buffer contents to remain unchanged, got %q", got)
	}
}
