package main

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/spf13/cobra"
)

type serviceAnnouncement struct {
	Path     string
	etcd     *etcd.Client
	Data     string
	TTL      uint64
	Interval time.Duration
}

func runAnnounce(cmd *cobra.Command, args []string) {

	if len(args) != 1 {
		log.Fatal("need a service name")
	}

	if announceTTL != 0 && announceTTL < announceInterval {
		log.Fatal("announce ttl must be greater than interval")
	}

	svc := strings.ToLower(args[0])

	// need better validation of name
	if len(svc) == 0 {
		log.Fatal("empty service name")
	}

	port := 0
	if len(args) >= 2 {
		var err error
		port, err = strconv.Atoi(args[1])
		if err != nil {
			log.Fatalf("failed to parse port: '%s' : %s", args[1], err)
		}
	}

	name, err := getNodeName()
	if err != nil {
		log.Fatal(err)
	}

	data, err := json.Marshal(&record{
		Port:   uint16(port),
		Target: name,
	})

	if err != nil {
		log.Fatal("json failure: %s", err)
	}

	a := &serviceAnnouncement{
		etcd:     etcd.NewClient(([]string{etcdAddress})),
		Path:     filepath.Join("/", etcdPrefix, "services", svc, name),
		Data:     string(data),
		TTL:      uint64(announceTTL),
		Interval: time.Duration(announceInterval) * time.Second,
	}

	a.announce()
	for _ = range time.Tick(a.Interval) {
		a.announce()
	}
}

//TODO: run check command
func (a *serviceAnnouncement) announce() {
	_, err := a.etcd.Set(a.Path, a.Data, a.TTL)
	if err != nil {
		log.Printf("failed to set %s : %s", a.Path, err)
	}
}
