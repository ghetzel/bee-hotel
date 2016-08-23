package bee

import (
	"testing"
)

func TestNewMultiClient(t *testing.T) {
	client := NewMultiClient(`1`, `2`)

	if v := client.Addresses[0]; v != `1` {
		t.Errorf("address 0: should be %q, is %q", `1`, v)
	}

	if v := client.Addresses[1]; v != `2` {
		t.Errorf("address 1: should be %q, is %q", `2`, v)
	}
}
