package main

import (
	"encoding/json"
	"log"
	"net"
	"path/filepath"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/spf13/cobra"
)

type nodeAnnouncement struct {
	IP   net.IP
	Path string
	etcd *etcd.Client
	Data string
}

func (a *nodeAnnouncement) announce() {
	_, err := a.etcd.Set(a.Path, a.Data, uint64(nodeTTL))
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
		log.Fatal("json failure: %s", err)
	}

	a := &nodeAnnouncement{
		etcd: etcd.NewClient(([]string{etcdAddress})),
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
