package chatcore

import "testing"

func TestCacheDrain(t *testing.T) {
	c := NewCache(2)
	c.Add("qq:-1", "alice", "one")
	c.Add("qq:-1", "bob", "two")
	c.Add("qq:-1", "carl", "three")

	got, n := c.Drain("qq:-1")
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	if got != "bob: two||carl: three" {
		t.Fatalf("drain = %q", got)
	}
	got, n = c.Drain("qq:-1")
	if got != "" || n != 0 {
		t.Fatalf("drain after clear = %q, %d", got, n)
	}
}
