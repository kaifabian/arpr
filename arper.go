package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/mdlayher/arp"
	"github.com/mdlayher/ethernet"
)

var (
	ifname = flag.String("i", "", "Network interface")
	ethaddr = flag.String("e", "", "Ethernet address")

	my_nets = []net.IPNet{}
	args = []string{}
)

func netsContain(nets []net.IPNet, ip net.IP) bool {
	for _, net := range nets {
		if net.Contains(ip) {
			return true
		}
	}

	return false
}

func usageError(errmsg string) {
	fmt.Fprintf(os.Stderr, "ERR Usage error: %s\n\n", errmsg)
	flag.Usage()
}

func main() {
	flag.Parse()
	args = flag.Args()

	if len(*ifname) == 0 {
		usageError("Must specify network interface (-i)!")
		return
	}

	if len(args) < 1 {
		usageError("Must specify at least 1 IP address or network!")
		return
	}

	for _, arg := range args {
		if !strings.Contains(arg, "/") {
			arg = fmt.Sprintf("%s/32", arg)
		}

		_, ipnet, err := net.ParseCIDR(arg)

		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Fprintf(os.Stderr, "INFO Listening for %s\n", ipnet.String())

		my_nets = append(my_nets, *ipnet)
	}

	// Get interface
	ifi, err := net.InterfaceByName(*ifname)

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR getInterface(%s): %s\n", ifname, err)
		return
	}

	// Deduce hardware address
	hwaddr := ifi.HardwareAddr

	if len(*ethaddr) > 0 {
		hwaddr, err = net.ParseMAC(*ethaddr)

		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR net.ParseMAC(%s): %s\n", *ethaddr, err)
			return
		}
	}

	// Open ARP client
	cli, err := arp.Dial(ifi)

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR arp.Dial(if): %s\n", err)
		return
	}

	// Do the job.
	for {
		pkt, eth, err := cli.Read()

		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR cli.Read: %s\n", err)
			continue
		}

		// Ignore packets that do not concern us
		if !bytes.Equal(eth.Destination, ethernet.Broadcast) && !bytes.Equal(eth.Destination, hwaddr) {
			continue
		}

		// Ignore packets that are no requsts
		if pkt.Operation != arp.OperationRequest {
			continue
		}

		// ARP request does not match our IP(s)
		if !netsContain(my_nets, pkt.TargetIP) {
			continue
		}

		err = cli.Reply(pkt, hwaddr, pkt.TargetIP)

		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR cli.Reply: %s\n", err)
			continue
		}
	}
}
