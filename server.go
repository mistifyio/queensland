package main

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/miekg/dns"
	"github.com/spf13/cobra"
)

type (
	server struct {
		etcd   *etcd.Client
		domain string
		prefix string
		ttl    uint32
	}
)

// DNS requester types
const (
	UnknownType = iota
	NodeType
	ServiceType
)

func nameError(w dns.ResponseWriter, req *dns.Msg) {
	m := &dns.Msg{}
	m.SetReply(req)
	m.SetRcode(req, dns.RcodeNameError)
	_ = w.WriteMsg(m)
}

func (s *server) GetNode(name string) (*node, error) {
	name = strings.ToLower(name)
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
		log.Printf("node %s has no address", name)
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

		target := strings.ToLower(record.Target)
		if !strings.Contains(target, ".") {
			target = strings.Join([]string{target, s.domain}, ".")
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

func toType(s string) int {
	switch s {
	case "services":
		return ServiceType
	case "nodes":
		return NodeType
	}
	return UnknownType
}

func checkName(qType uint16, name string) (string, int) {
	parts := strings.Split(name, ".")
	parts = parts[:len(parts)-1]

	switch len(parts) {
	case 2:
		return parts[0], toType(parts[1])
	case 3:
		// horrible hack to munge _service._protocol.services.<domain> queries

		if qType == dns.TypeSRV && parts[2] == "services" && (parts[1] == "_tcp" || parts[1] == "_udp") {
			n := parts[0]
			if len(n) > 1 && (string([]rune(n)[0]) == "_") {
				return strings.TrimPrefix(n, "_"), ServiceType
			}
		}
	}
	// just horrible...
	return "", UnknownType
}

func (s *server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {

	question := strings.ToLower(r.Question[0].Name)
	name := strings.TrimSuffix(question, s.domain)

	qType := r.Question[0].Qtype

	name, rType := checkName(qType, name)

	if name == "" || rType == UnknownType {
		log.Printf("invalid query: %s", question)
		nameError(w, r)
		return
	}

	var m *dns.Msg
	var err error
	switch rType {
	case ServiceType:
		switch qType {
		case dns.TypeA:
			m, err = s.ServicesA(w, r, name)
		case dns.TypeSRV:
			m, err = s.ServicesSRV(w, r, name)
		}
	case NodeType:
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

// TODO: start a simple http status interface.  should be a different service?

func runServer(cmd *cobra.Command, args []string) {
	e := etcd.NewClient(([]string{etcdAddress}))

	parts := strings.Split(dnsDomain, ".")
	dom := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) > 0 {
			dom = append(dom, p)
		}
	}

	s := &server{
		etcd:   e,
		domain: strings.ToLower(strings.Join(dom, ".") + "."),
		prefix: etcdPrefix,
		ttl:    uint32(dnsTTL),
	}

	dns.Handle(s.domain, s)

	server := &dns.Server{
		Addr:         fmt.Sprintf(":%d", dnsPort),
		Net:          "udp",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}
