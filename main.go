package main

import (
	"net"

	logx "github.com/mistifyio/mistify-logrus-ext"
	"github.com/spf13/cobra"
)

type (
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

func help(cmd *cobra.Command, _ []string) {
	_ = cmd.Help()
}

var (
	announceCheck    string
	announceInterval uint
	announcePort     uint
	announceWeight   uint
	announcePriority uint
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
	removeOnExit     bool
)

func main() {
	_ = logx.DefaultSetup("error")

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

	cmdAnnounce.PersistentFlags().StringVarP(&announceCheck, "check", "c", "", "announce check command")
	cmdAnnounce.PersistentFlags().StringVarP(&nodeName, "name", "n", "", "node name. will default to hostname.")
	cmdAnnounce.PersistentFlags().UintVarP(&announceInterval, "interval", "i", 30, "announce interval")
	cmdAnnounce.PersistentFlags().UintVarP(&announcePort, "port", "o", 0, "announce service port")
	cmdAnnounce.PersistentFlags().UintVarP(&announcePriority, "priority", "r", 0, "announce service priority")
	cmdAnnounce.PersistentFlags().UintVarP(&announceTTL, "ttl", "t", 60, "announce ttl. 0 disables")
	cmdAnnounce.PersistentFlags().UintVarP(&announceWeight, "weight", "w", 0, "announce service weight")
	cmdAnnounce.PersistentFlags().BoolVarP(&removeOnExit, "remove", "", false, "remove key on exit")

	cmdNode := &cobra.Command{
		Use:   "node",
		Short: "announce the node",
		Run:   runNode,
	}

	cmdNode.PersistentFlags().UintVarP(&nodeTTL, "ttl", "t", 0, "announce ttl.  0 disables")
	cmdNode.PersistentFlags().UintVarP(&nodeInterval, "interval", "i", 300, "announce interval")
	cmdNode.PersistentFlags().StringVarP(&nodeName, "name", "n", "", "node name. will default to hostname.")
	cmdNode.PersistentFlags().StringVarP(&nodeIP, "address", "a", "", "node addresses. will default to first non-localhost IP")
	cmdNode.PersistentFlags().BoolVarP(&removeOnExit, "remove", "", false, "remove key on exit")

	root.AddCommand(
		cmdServer,
		cmdAnnounce,
		cmdNode,
	)
	_ = root.Execute()

}
