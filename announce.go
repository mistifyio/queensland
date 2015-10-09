package main

import (
	"encoding/json"
	"os/exec"
	"path"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/go-etcd/etcd"
	"github.com/spf13/cobra"
)

type serviceAnnouncement struct {
	Path     string
	etcd     *etcd.Client
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
		log.WithFields(log.Fields{
			"announceTTL":      announceTTL,
			"announceInterval": announceInterval,
		}).Fatal("announce ttl must be greater than interval")
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

	r := &record{
		Port:     uint16(announcePort),
		Weight:   uint16(announceWeight),
		Target:   name,
		Priority: uint16(announcePriority),
	}
	data, err := json.Marshal(r)

	if err != nil {
		log.WithFields(log.Fields{
			"record": r,
			"error":  err,
		}).Fatal("failed to marshal record json")
	}

	a := &serviceAnnouncement{
		Check:    announceCheck,
		Data:     string(data),
		Interval: time.Duration(announceInterval) * time.Second,
		Path:     path.Join("/", etcdPrefix, "services", svc, name),
		TTL:      uint64(announceTTL),
		etcd:     etcd.NewClient(([]string{etcdAddress})),
	}

	handleRemoveOnExit(a.etcd, a.Path)

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
			log.WithFields(log.Fields{
				"check":  a.Check,
				"error":  err,
				"output": output,
			}).Error("failed to run check")
			return
		}
	}

	_, err := a.etcd.Set(a.Path, a.Data, a.TTL)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  a.Path,
			"error": err,
		}).Error("failed to set path data")
	}
}
