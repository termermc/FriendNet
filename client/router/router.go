package router

import (
	"context"
	"errors"
	"net"

	"github.com/huin/goupnp/dcps/internetgateway2"
	"golang.org/x/sync/errgroup"
)

type Client interface {
	AddPortMapping(
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
		NewInternalPort uint16,
		NewInternalClient string,
		NewEnabled bool,
		NewPortMappingDescription string,
		NewLeaseDuration uint32,
	) (err error)

	GetExternalIPAddress() (
		NewExternalIPAddress string,
		err error,
	)

	LocalAddr() net.IP
}

func PickRouterClient(ctx context.Context) (Client, error) {
	tasks, _ := errgroup.WithContext(ctx)

	// Request each type of client in parallel, and return what is found.
	var ip1Clients []*internetgateway2.WANIPConnection1
	tasks.Go(func() error {
		var err error
		ip1Clients, _, err = internetgateway2.NewWANIPConnection1Clients()
		return err
	})
	var ip2Clients []*internetgateway2.WANIPConnection2
	tasks.Go(func() error {
		var err error
		ip2Clients, _, err = internetgateway2.NewWANIPConnection2Clients()
		return err
	})
	var ppp1Clients []*internetgateway2.WANPPPConnection1
	tasks.Go(func() error {
		var err error
		ppp1Clients, _, err = internetgateway2.NewWANPPPConnection1Clients()
		return err
	})

	if err := tasks.Wait(); err != nil {
		return nil, err
	}

	// Trivial handling for where we find exactly one device to talk to, you
	// might want to provide more flexible handling than this if multiple
	// devices are found.
	switch {
	case len(ip2Clients) == 1:
		return ip2Clients[0], nil
	case len(ip1Clients) == 1:
		return ip1Clients[0], nil
	case len(ppp1Clients) == 1:
		return ppp1Clients[0], nil
	default:
		return nil, errors.New("multiple or no services found")
	}
}

// GetIpAndForwardPort gets the external IP address of the router and forwards a port.
// The port will be both the internal port and the external Internet-exposed port.
func GetIpAndForwardPort(ctx context.Context, port uint16) (externalIp string, err error) {
	client, err := PickRouterClient(ctx)
	if err != nil {
		return "", err
	}

	externalIp, err = client.GetExternalIPAddress()
	if err != nil {
		return "", err
	}

	err = client.AddPortMapping(
		"",
		// External port number to expose to Internet:
		port,
		// Forward TCP (this could be "UDP" if we wanted that instead).
		"TCP",
		// Internal port number on the LAN to forward to.
		// Some routers might not support this being different to the external
		// port number.
		port,
		// Internal address on the LAN we want to forward to.
		client.LocalAddr().String(),
		// Enabled:
		true,
		// Informational description for the client requesting the port forwarding.
		"FriendNet Direct Connection",
		// How long should the port forward last for in seconds.
		// If you want to keep it open for longer and potentially across router
		// resets, you might want to periodically request before this elapses.
		3600,
	)
	if err != nil {
		return "", err
	}

	return externalIp, nil
}
