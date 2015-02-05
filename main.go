package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"path/filepath"
	"strings"
	"time"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/go-etcd/etcd"
	"github.com/miekg/dns"
)

type (
	server struct {
		etcd   *etcd.Client
		domain string
		prefix string
		ttl    uint32
	}

	record struct {
		Priority uint16 `json:"priority"`
		Weight   uint16 `json:"weight"`
		Port     uint16 `json:"port"`
		Target   string `json:"target"`
	}

	node struct {
		IP net.IP `json:"ip"`
	}
)

func isKeyNotFound(err error) bool {
	e, ok := err.(*etcd.EtcdError)
	return ok && e.ErrorCode == etcdErr.EcodeKeyNotFound
}

func nameError(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.SetRcode(req, dns.RcodeNameError)
	_ = w.WriteMsg(m)
}

func (s *server) GetNode(name string) (*node, error) {
	name = strings.TrimSuffix(name, ".nodes.")
	key := filepath.Join("/", s.prefix, "nodes", name)

	resp, err := s.etcd.Get(key, false, false)

	if err != nil {
		return nil, err
	}

	var n node
	err = json.Unmarshal([]byte(resp.Node.Value), &n)
	if err != nil {
		return nil, err
	}

	if n.IP == nil {
		return nil, nil
	}
	return &n, nil
}

func (s *server) GetService(name string) ([]*record, error) {
	name = strings.TrimSuffix(name, ".services.")

	key := filepath.Join("/", s.prefix, "services", name)

	resp, err := s.etcd.Get(key, false, true)

	if err != nil {
		return nil, err
	}

	records := make([]*record, 0, len(resp.Node.Nodes))

	for _, n := range resp.Node.Nodes {
		var rec record
		err := json.Unmarshal([]byte(n.Value), &rec)
		if err != nil {
			log.Printf("json.Unmarshal failed for %s: %s", n.Key, err)
			continue
		}

		// should match against a regex?
		if rec.Target == "" {
			continue
		}

		records = append(records, &rec)
	}

	return records, nil
}

func (s *server) ServicesA(w dns.ResponseWriter, r *dns.Msg, name string) (*dns.Msg, error) {
	records, err := s.GetService(name)

	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}
	m := &dns.Msg{}
	m.SetReply(r)

	header := dns.RR_Header{
		Name:   r.Question[0].Name,
		Rrtype: r.Question[0].Qtype,
		Class:  r.Question[0].Qclass,
		Ttl:    s.ttl,
	}

	m.Answer = make([]dns.RR, 0, len(records))
	for _, record := range records {
		// any target that contains a "." is assumed to not be a node
		if strings.Contains(record.Target, ".") {
			continue
		}

		node, err := s.GetNode(record.Target)
		if err != nil {
			continue
		}

		answer := &dns.A{
			Hdr: header,
			A:   node.IP,
		}

		m.Answer = append(m.Answer, answer)
	}

	return m, nil

}

func (s *server) ServicesSRV(w dns.ResponseWriter, r *dns.Msg, name string) (*dns.Msg, error) {
	records, err := s.GetService(name)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	m := &dns.Msg{}
	m.SetReply(r)

	header := dns.RR_Header{
		Name:   r.Question[0].Name,
		Rrtype: r.Question[0].Qtype,
		Class:  r.Question[0].Qclass,
		Ttl:    s.ttl,
	}

	m.Answer = make([]dns.RR, 0, len(records))
	m.Extra = make([]dns.RR, 0, len(records))
	for _, record := range records {

		isNode := false

		target := record.Target
		if !strings.Contains(target, ".") {
			target = strings.Join([]string{record.Target, s.domain}, ".")
			isNode = true
			// should we make sure it exists before we add it?
		}

		answer := &dns.SRV{
			Hdr:      header,
			Priority: record.Priority,
			Weight:   record.Weight,
			Port:     record.Port,
			Target:   target,
		}

		m.Answer = append(m.Answer, answer)

		if !isNode {
			continue
		}

		node, err := s.GetNode(record.Target)
		if err != nil {
			// just skip it
			continue
		}

		extra := &dns.A{
			Hdr: dns.RR_Header{
				Name:   target,
				Rrtype: dns.TypeA,
				Class:  r.Question[0].Qclass,
				Ttl:    s.ttl,
			},
			A: node.IP,
		}

		m.Extra = append(m.Extra, extra)
	}

	return m, nil

}

func (s *server) NodesA(w dns.ResponseWriter, r *dns.Msg, name string) (*dns.Msg, error) {
	node, err := s.GetNode(name)

	if err != nil {
		return nil, err
	}

	m := &dns.Msg{}
	m.SetReply(r)

	header := dns.RR_Header{
		Name:   r.Question[0].Name,
		Rrtype: r.Question[0].Qtype,
		Class:  r.Question[0].Qclass,
		Ttl:    s.ttl,
	}

	m.Answer = []dns.RR{
		&dns.A{
			Hdr: header,
			A:   node.IP,
		},
	}

	return m, nil

}
func (s *server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {

	question := strings.ToLower(r.Question[0].Name)
	name := strings.TrimSuffix(question, s.domain)
	parts := strings.Split(name, ".")
	parts = parts[:len(parts)-1]

	if len(parts) != 2 {
		log.Printf("invalid query: %s", question)
		nameError(w, r)
		return
	}

	rType := parts[1]
	qType := r.Question[0].Qtype

	var m *dns.Msg
	var err error
	switch rType {
	case "services":
		switch qType {
		case dns.TypeA:
			m, err = s.ServicesA(w, r, name)
		case dns.TypeSRV:
			m, err = s.ServicesSRV(w, r, name)
		}
	case "nodes":
		switch qType {
		case dns.TypeA:
			m, err = s.NodesA(w, r, name)
		}
	}

	if err != nil {
		log.Println(err)
		if isKeyNotFound(err) {
			nameError(w, r)
		} else {
			dns.HandleFailed(w, r)
		}
		return
	}

	if m == nil {
		log.Printf("%s: not found", question)
		nameError(w, r)
		return
	}

	_ = w.WriteMsg(m)
}

func main() {

	ttl := flag.Uint("ttl", 0, "DNS TTL for responses")
	domain := flag.String("domain", "local.", "domain - must end with '.'")
	eaddr := flag.String("etcd", "http://localhost:4001", "etcd address")
	prefix := flag.String("prefix", "/", "etcd prefix")
	address := flag.String("address", ":15353", "UDP address to listen")
	flag.Parse()

	e := etcd.NewClient(([]string{*eaddr}))

	s := &server{
		etcd:   e,
		domain: *domain,
		prefix: *prefix,
		ttl:    uint32(*ttl),
	}

	dns.Handle(s.domain, s)

	server := &dns.Server{
		Addr:         *address,
		Net:          "udp",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Fatal(server.ListenAndServe())

}
