package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/coreos/etcd/client"
	etcdErr "github.com/coreos/etcd/error"
)

func isKeyNotFound(err error) bool {
	e, ok := err.(*client.Error)
	return ok && e.Code == etcdErr.EcodeKeyNotFound
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
			if ip.To4() == nil {
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

	// XXX: we currently only correctly handle v4
	if ip.To4() == nil {
		return nil, fmt.Errorf("not an ipv4 address: %s", nodeIP)
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

func handleRemoveOnExit(e *etcd.Client, key string) {
	if removeOnExit {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			for _ = range c {
				_, err := e.Delete(key, false)
				if err != nil {
					log.Printf("delete of '%s' failed: %s", key, err)
				}
				os.Exit(0)
			}
		}()
	}
}
