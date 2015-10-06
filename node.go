package main

import (
	"encoding/json"
	"net"
	"path/filepath"
	"time"

	log "github.com/Sirupsen/logrus"
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
		log.WithFields(log.Fields{
			"path":  a.Path,
			"data":  a.Data,
			"error": err,
		}).Error("failed to set path data")
	}
}

func runNode(cmd *cobra.Command, args []string) {

	name, err := getNodeName()
	if err != nil {
		log.WithField("error", err).Fatal("failed to get node name")
	}

	ip, err := getNodeIP()
	if err != nil {
		log.WithFields(log.Fields{
			"name":  name,
			"error": err,
		}).Fatal("failed to get node ip")
	}

	n := &node{IP: ip}
	data, err := json.Marshal(n)

	if err != nil {
		log.WithFields(log.Fields{
			"name":  name,
			"node":  n,
			"error": err,
		}).Fatal("failed to json marshal node")
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
