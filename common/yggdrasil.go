package common

import "net/netip"

// YggdrasilPrefix is the prefix for Yggdrasil addresses.
var YggdrasilPrefix = netip.MustParsePrefix("0200::/7")
