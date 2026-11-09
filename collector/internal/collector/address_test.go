package collector

import (
	"net/netip"
	"testing"
)

func TestIsPublicAddress(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"169.254.1.1", false},
		{"192.0.2.1", false},
		{"::1", false},
		{"fe80::1", false},
	}
	for _, test := range tests {
		address := netip.MustParseAddr(test.value)
		if got := IsPublicAddress(address); got != test.want {
			t.Errorf("IsPublicAddress(%s) = %t, want %t", test.value, got, test.want)
		}
	}
}
