package collector

import (
	"fmt"
	"net/netip"
)

// IsPublicAddress accepts only globally routable unicast addresses. Callers must
// run this check after every DNS resolution, including immediately before dialing.
func IsPublicAddress(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsPrivate() || address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	if address.Is4In6() {
		address = address.Unmap()
	}
	if address.Is4() {
		for _, prefix := range blockedIPv4Prefixes {
			if prefix.Contains(address) {
				return false
			}
		}
	}
	return true
}

func MustPrefix(value string) netip.Prefix {
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		panic(fmt.Sprintf("invalid static prefix %q: %v", value, err))
	}
	return prefix
}

var blockedIPv4Prefixes = []netip.Prefix{
	MustPrefix("0.0.0.0/8"),
	MustPrefix("100.64.0.0/10"),
	MustPrefix("127.0.0.0/8"),
	MustPrefix("169.254.0.0/16"),
	MustPrefix("192.0.0.0/24"),
	MustPrefix("192.0.2.0/24"),
	MustPrefix("198.18.0.0/15"),
	MustPrefix("198.51.100.0/24"),
	MustPrefix("203.0.113.0/24"),
	MustPrefix("224.0.0.0/4"),
	MustPrefix("240.0.0.0/4"),
}
