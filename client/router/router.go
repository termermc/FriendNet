package router

import (
	"context"
	"errors"
	"fmt"
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

func routerClients(ctx context.Context) ([]Client, error) {
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

	clients := make([]Client, 0, len(ip2Clients)+len(ip1Clients)+len(ppp1Clients))
	for _, client := range ip2Clients {
		clients = append(clients, client)
	}
	for _, client := range ip1Clients {
		clients = append(clients, client)
	}
	for _, client := range ppp1Clients {
		clients = append(clients, client)
	}

	return clients, nil
}

// GetIpAndForwardPort gets the external IP address of the router and forwards a port.
// The port will be both the internal port and the external Internet-exposed port.
func GetIpAndForwardPort(ctx context.Context, port uint16) (externalIp string, err error) {
	clients, err := routerClients(ctx)
	if err != nil {
		return "", err
	}
	if len(clients) == 0 {
		return "", errors.New("no services found")
	}

	var errs []error
	for _, client := range clients {
		externalIp, err = client.GetExternalIPAddress()
		if err != nil {
			errs = append(errs, fmt.Errorf("GetExternalIPAddress via %T failed: %w", client, err))
			continue
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
			errs = append(errs, fmt.Errorf("AddPortMapping via %T failed: %w", client, err))
			continue
		}

		return externalIp, nil
	}

	if len(errs) == 0 {
		return "", errors.New("no services found")
	}
	return "", errors.Join(errs...)
}
