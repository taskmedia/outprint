package outline

import "net/netip"

// blockedPrefixes are address ranges an image fetch must never reach unless
// the address belongs to the configured Outline origin: everything that is
// not the public internet, plus cloud metadata endpoints.
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",        // "this network"
	"100.64.0.0/10",    // carrier-grade NAT, also some cloud metadata services
	"192.0.0.0/24",     // IETF protocol assignments
	"192.0.2.0/24",     // documentation
	"198.18.0.0/15",    // benchmarking
	"198.51.100.0/24",  // documentation
	"203.0.113.0/24",   // documentation
	"240.0.0.0/4",      // reserved, includes the broadcast address
	"168.63.129.16/32", // cloud platform wire server
	"64:ff9b::/96",     // NAT64: embeds an IPv4 address
	"2001::/32",        // Teredo: embeds an IPv4 address
	"2001:db8::/32",    // documentation
	"2002::/16",        // 6to4: embeds an IPv4 address
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

// blockedAddr reports whether an image fetch must refuse the address:
// loopback, private, link-local (which holds the cloud metadata address),
// unspecified, multicast and the ranges above. An IPv4 address written as
// IPv6 is judged as the IPv4 address it is.
func blockedAddr(a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	switch {
	case !a.IsValid(), a.IsLoopback(), a.IsPrivate(), a.IsUnspecified(), a.IsMulticast(),
		a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast(), a.IsInterfaceLocalMulticast():
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
