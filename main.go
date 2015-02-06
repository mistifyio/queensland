package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	etcdErr "github.com/coreos/etcd/error"
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

	record struct {
		Priority uint16 `json:"priority"`
		Weight   uint16 `json:"weight"`
		Port     uint16 `json:"port"`
		Target   string `json:"target"`
	}

	node struct {
		IP net.IP `json:"ip"`
	}

	nodeAnnouncement struct {
		IP   string
		Path string
		etcd *etcd.Client
		Data string
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

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}

var (
	announceCheck    string
	announceInterval uint
	announceName     string
	announceTTL      uint
	dnsDomain        string
	dnsPort          uint
	dnsTTL           uint
	etcdAddress      string
	etcdPrefix       string
	httpPort         uint
	nodeInterval     uint
	nodeIP           string
	nodeName         string
	nodeTTL          uint
)

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

func runAnnounce(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		log.Fatal("need a service name")
	}

	if announceTTL != 0 && announceTTL < announceInterval {
		log.Fatal("announce ttl must be greater than interval")
	}

	// run in a loop doing a PUT every interval seconds with ttl
	// optionally running the check comamnd. only announce if check command passes
	// remove on exit (flag for this?)
}

func (a *nodeAnnouncement) announce() {
	_, err := a.etcd.Set(a.Path, a.Data, uint64(nodeTTL))
	if err != nil {
		log.Printf("failed to set %s : %s", a.Path, err)
	}
}

func runNode(cmd *cobra.Command, args []string) {
	if nodeName == "" {
		var err error
		nodeName, err = os.Hostname()
		if err != nil {
			log.Fatalf("failed to get hostname: %s", err)
		}
	}

	if nodeIP == "" {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			log.Fatalf("failed to get interface addresses: %s", err)
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
		log.Fatal("failed to get address")
	}

	ip := net.ParseIP(nodeIP)
	if ip == nil {
		log.Fatalf("failed to parse address: %s", nodeIP)
	}

	data, err := json.Marshal(&node{
		IP: ip,
	})

	if err != nil {
		log.Fatal("apparently Freddy won")
	}

	a := &nodeAnnouncement{
		etcd: etcd.NewClient(([]string{etcdAddress})),
		Path: filepath.Join("/", etcdPrefix, "nodes", nodeName),
		IP:   nodeIP,
		Data: string(data),
	}

	a.announce()
	for _ = range time.Tick(time.Duration(nodeInterval) * time.Second) {
		a.announce()
	}
}

func main() {

	root := &cobra.Command{
		Use:  "queensland",
		Long: "queensland is a simple service discovery DNS server for etcd",
		Run:  help,
	}

	root.PersistentFlags().StringVarP(&etcdAddress, "etcd", "e", "http://127.0.0.1:4001", "etcd endpoint")
	root.PersistentFlags().StringVarP(&etcdPrefix, "prefix", "p", "/queensland", "etcd prefix")

	cmdServer := &cobra.Command{
		Use:   "server",
		Short: "Run as DNS server",
		Run:   runServer,
	}
	cmdServer.PersistentFlags().UintVarP(&dnsTTL, "ttl", "t", 0, "dns ttl")
	cmdServer.PersistentFlags().UintVarP(&dnsPort, "dns-port", "d", 15353, "dns server port")
	cmdServer.PersistentFlags().UintVarP(&httpPort, "http-port", "o", 25353, "http server port")
	cmdServer.PersistentFlags().StringVarP(&dnsDomain, "domain", "m", ".local", "dns domain")

	cmdAnnounce := &cobra.Command{
		Use:   "announce name",
		Short: "service announce",
		Run:   runAnnounce,
	}

	cmdAnnounce.PersistentFlags().UintVarP(&announceTTL, "ttl", "t", 60, "announce ttl. 0 disables")
	cmdAnnounce.PersistentFlags().UintVarP(&announceInterval, "interval", "i", 30, "announce interval")
	cmdAnnounce.PersistentFlags().StringVarP(&announceCheck, "check", "c", "", "announce check command")
	cmdAnnounce.PersistentFlags().StringVarP(&announceName, "name", "n", "", "node name. will default to hostname.")

	cmdNode := &cobra.Command{
		Use:   "node",
		Short: "announce the node",
		Run:   runNode,
	}

	cmdNode.PersistentFlags().UintVarP(&nodeTTL, "ttl", "t", 0, "announce ttl.  0 disables")
	cmdNode.PersistentFlags().UintVarP(&nodeInterval, "interval", "i", 300, "announce interval")
	cmdNode.PersistentFlags().StringVarP(&nodeName, "name", "n", "", "node name. will default to hostname.")
	cmdNode.PersistentFlags().StringVarP(&nodeIP, "address", "a", "", "node addresses. will default to first non-localhost IP")

	root.AddCommand(
		cmdServer,
		cmdAnnounce,
		cmdNode,
	)
	root.Execute()

}
