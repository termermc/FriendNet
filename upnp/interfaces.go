// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Apparently there is a bug with this on Android.
// There was another implementation of this module using github.com/wlynxg/anet to fix it,
// but I don't want to add another dependency for a platform this will almost certainly
// never run on.

//go:build !android

package upnp

import "net"

func listInterfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

func interfaceAddrsByInterface(intf *net.Interface) ([]net.Addr, error) {
	return intf.Addrs()
}
