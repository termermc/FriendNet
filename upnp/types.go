// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upnp

import (
	"context"
	"net"
	"strconv"
	"time"
)

type Protocol string

const (
	TCP Protocol = "TCP"
	UDP Protocol = "UDP"
)

type IPVersion int8

const (
	IPvAny IPVersion = iota
	IPv4Only
	IPv6Only
)

// Address is essentially net.TCPAddr yet is more general, and has a few helper
// methods which reduce boilerplate code.
type Address struct {
	IP   net.IP
	Port int
}

func (a Address) Equal(b Address) bool {
	return a.Port == b.Port && a.IP.Equal(b.IP)
}

func (a Address) String() string {
	var ipStr string
	if a.IP == nil {
		ipStr = net.IPv4zero.String()
	} else {
		ipStr = a.IP.String()
	}
	return net.JoinHostPort(ipStr, strconv.Itoa(a.Port))
}

func (a Address) GoString() string {
	return a.String()
}

// Device describes the functionality exposed by an IGD service.
type Device interface {
	ID() string
	GetLocalIPv4Address() net.IP
	AddPortMapping(ctx context.Context, protocol Protocol, internalPort, externalPort int, description string, duration time.Duration) (int, error)
	AddPinhole(ctx context.Context, protocol Protocol, addr Address, duration time.Duration) ([]net.IP, error)
	GetExternalIPv4Address(ctx context.Context) (net.IP, error)
	SupportsIPVersion(version IPVersion) bool
}
