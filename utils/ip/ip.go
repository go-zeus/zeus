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

// LocalIP 返回本机首个非 loopback IPv4 地址
// 探测失败时返回 127.0.0.1（保证不 panic，但调用方应意识到可能注册不可达地址）
func LocalIP() string {
	ips, err := LocalIPv4s()
	if err != nil || len(ips) == 0 {
		return "127.0.0.1"
	}
	return ips[0]
}
