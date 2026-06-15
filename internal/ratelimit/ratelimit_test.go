package ratelimit

import "testing"

func TestAllowWithinBurst(t *testing.T) {
	l := New()
	// burst of 3 -> first 3 immediate calls allowed, 4th denied
	for i := 0; i < 3; i++ {
		if !l.Allow("k", 1, 3) {
			t.Fatalf("call %d should be allowed", i)
		}
	}
	if l.Allow("k", 1, 3) {
		t.Fatal("4th call should be denied")
	}
}

func TestSeparateKeys(t *testing.T) {
	l := New()
	if !l.Allow("a", 1, 1) {
		t.Fatal("a first call allowed")
	}
	if !l.Allow("b", 1, 1) {
		t.Fatal("b first call allowed (independent bucket)")
	}
}
