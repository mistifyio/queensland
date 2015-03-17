package main

import (
	"encoding/json"
	"log"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

type serviceAnnouncement struct {
	Path     string
	etcd     client.Client
	Data     string
	TTL      uint64
	Interval time.Duration
	Check    string
}

// XXX: when process exits should we remove the key from etcd? configurable via flag?

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

	name, err := getNodeName()
	if err != nil {
		log.Fatal(err)
	}

	data, err := json.Marshal(&record{
		Port:     uint16(announcePort),
		Weight:   uint16(announceWeight),
		Target:   name,
		Priority: uint16(announcePriority),
	})

	if err != nil {
		log.Fatal("json failure: %s", err)
	}

	cfg := client.Config{
		Endpoints: []string{etcdAddress},
		Transport: client.DefaultTransport,
	}
	c, err := client.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	a := &serviceAnnouncement{
		Check:    announceCheck,
		Data:     string(data),
		Interval: time.Duration(announceInterval) * time.Second,
		Path:     path.Join("/", etcdPrefix, "services", svc, name),
		TTL:      uint64(announceTTL),
		etcd:     c,
	}

	a.announce()
	for _ = range time.Tick(a.Interval) {
		a.announce()
	}
}

func (a *serviceAnnouncement) announce() {

	if a.Check != "" {
		// should we wrap in a timeout?
		c := exec.Command("/bin/sh", "-c", a.Check)
		output, err := c.CombinedOutput()
		if err != nil {
			// should failure immediately remove the entry or should we let ttl timeout?
			// do rise/fall style checks?
			log.Printf("failed to run '%s' : %s : '%s'", a.Check, err, output)
			return
		}
	}

	kAPI := client.NewKeysAPI(a.etcd)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := kAPI.Set(ctx, a.Path, a.Data, &client.SetOptions{TTL: time.Duration(nodeTTL) * time.Second})
	if err != nil {
		log.Printf("failed to set %s : %s", a.Path, err)
	}

}
