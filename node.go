package main

import (
	"encoding/json"
	"log"
	"net"
	"path/filepath"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

type nodeAnnouncement struct {
	IP   net.IP
	Path string
	etcd client.Client
	Data string
}

func (a *nodeAnnouncement) announce() {

	kAPI := client.NewKeysAPI(a.etcd)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := kAPI.Set(ctx, a.Path, a.Data, &client.SetOptions{TTL: time.Duration(nodeTTL) * time.Second})
	if err != nil {
		log.Printf("failed to set %s : %s", a.Path, err)
	}

}

func runNode(cmd *cobra.Command, args []string) {

	name, err := getNodeName()
	if err != nil {
		log.Fatal(err)
	}

	ip, err := getNodeIP()
	if err != nil {
		log.Fatal(err)
	}

	data, err := json.Marshal(&node{
		IP: ip,
	})

	if err != nil {
		log.Fatalf("json failure: %s\n", err)
	}

	cfg := client.Config{
		Endpoints: []string{etcdAddress},
		Transport: client.DefaultTransport,
	}
	c, err := client.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	a := &nodeAnnouncement{
		etcd: c,
		Path: filepath.Join("/", etcdPrefix, "nodes", name),
		IP:   ip,
		Data: string(data),
	}

	handleRemoveOnExit(a.etcd, a.Path)

	a.announce()
	for _ = range time.Tick(time.Duration(nodeInterval) * time.Second) {
		a.announce()
	}
}
