package consul

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/hashicorp/consul/api"
)

// TODO: meigas copied from command/connect/envoy/flags.go
func parseAddress(raw string) (api.ServiceAddress, error) {
	result := api.ServiceAddress{}
	addr, portStr, err := net.SplitHostPort(raw)
	// Error message from Go's net/ipsock.go
	if err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return result, fmt.Errorf("error parsing address %q: %v", raw, err)
		}

		// Use the whole input as the address if there wasn't a port.
		if ip := net.ParseIP(raw); ip == nil {
			return result, fmt.Errorf("error parsing address %q: not an IP address", raw)
		}
		addr = raw
	}

	port := 8443
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return result, fmt.Errorf("error parsing port %q: %v", portStr, err)
		}
	}

	result.Address = addr
	result.Port = port
	return result, nil
}
