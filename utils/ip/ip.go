package ip

import (
	"net"
)

// LocalIPv4s return all non-loopback IPv4 addresses
func LocalIPv4s() ([]string, error) {
	var ips []string
	adders, err := net.InterfaceAddrs()
	if err != nil {
		return ips, err
	}

	for _, a := range adders {
		if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			ips = append(ips, ipNet.IP.String())
		}
	}

	return ips, nil
}

func LocalIP() string {
	ips, _ := LocalIPv4s()
	return ips[0]
}
