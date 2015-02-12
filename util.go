package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/go-etcd/etcd"
)

func isKeyNotFound(err error) bool {
	e, ok := err.(*etcd.EtcdError)
	return ok && e.ErrorCode == etcdErr.EcodeKeyNotFound
}

func getNodeIP() (net.IP, error) {

	if nodeIP == "" {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return nil, fmt.Errorf("failed to get interface addresses: %s", err)
		}

		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil {
				// log error?
				continue
			}
			if ip.IsGlobalUnicast() {
				nodeIP = ip.String()
				break
			}
		}
	}

	if nodeIP == "" {
		return nil, fmt.Errorf("failed to get address")
	}

	ip := net.ParseIP(nodeIP)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse address: %s", nodeIP)
	}

	return ip, nil

}

func getNodeName() (string, error) {
	if nodeName == "" {
		var err error
		nodeName, err = os.Hostname()
		if err != nil {
			return "", fmt.Errorf("failed to get hostname: %s", err)
		}
	}

	return strings.ToLower(nodeName), nil

}
