package fixture

import "testing"

func TestPayloadCorrupt(t *testing.T) {
	// A corrupt length: 128 TiB.
	rec := []byte{0, 0, 0x80, 0, 0, 0, 0, 0, 'h', 'i'}
	if got := Payload(rec); len(got) != 2 {
		t.Errorf("len(Payload) = %d, want 2", len(got))
	}
}
