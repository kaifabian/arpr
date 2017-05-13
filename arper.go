package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/arp"
	"github.com/mdlayher/ethernet"
)

var (
	ifname  = flag.String("i", "", "Network interface")
	ethaddr = flag.String("e", "", "Ethernet address")
	gratArp = flag.Bool("g", false, "Send gratuitous ARP")
	gratInt = flag.Int("G", 60, "Interval to send gratuitous ARP in [seconds]")
	gratMax = flag.Int("M", 1024, "Maximum number of IP addresses for gratuitous ARP (performance implications!)")

	myNets = []net.IPNet{}
	args   = []string{}
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

func gratuitousArp(cli *arp.Client, ip net.IP, hwaddr net.HardwareAddr, sleepInt time.Duration) {
	hwBcast, err := net.ParseMAC("ff-ff-ff-ff-ff-ff")
	gratArpPkt, err := arp.NewPacket(arp.OperationReply, hwaddr, ip, hwBcast, ip)

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR arp.NewPacket: %s", err)
		return
	}

	for {
		err = cli.WriteTo(gratArpPkt, hwBcast)

		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR cli.WriteTo: %s", err)
			return
		}

		time.Sleep(sleepInt)
	}
}

func incrementIP(ip *net.IP) {
	for i := int(len(*ip)) - 1; i >= 0; i-- {
		if (*ip)[i] >= 255 {
			(*ip)[i] = 0;
			continue;
		} else {
			(*ip)[i] += 1;
			break;
		}
	}
}

func allIps(ipNet net.IPNet, ch chan net.IP) {
	ch <- ipNet.IP

	ones, bits := ipNet.Mask.Size() // IPMask.Size assumes canonical netmask -- as do we

	if bits == 0 {
		// Non-canonical IP
		return
	}

	numIps := 1 << uint(bits - ones)
	current := net.IP(ipNet.IP)

	for i := 0; i < numIps; i++ {
		ch <- current
		incrementIP(&current)
	}
}

func AllIps(ipNets []net.IPNet) <-chan net.IP {
	ch := make(chan net.IP)
	go func () {
		for _, ipNet := range ipNets {
			allIps(ipNet, ch)
		}
	} ();
	return ch
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

		myNets = append(myNets, *ipnet)
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

	numAddr := 0
	for _, net := range myNets {
		ones, bits := net.Mask.Size()
		numAddr += 1 << uint(bits - ones)
	}

	if *gratArp {
		if numAddr > *gratMax {
			usageError("Too many IP addresses for gratuitous ARP -- either decrease number of IP addresses or increase -M!")
			return
		}

		sleepInt := time.Duration(*gratInt) * time.Second

		for ip := range AllIps(myNets) {
			go gratuitousArp(cli, ip, hwaddr, sleepInt)
		}
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
		if !netsContain(myNets, pkt.TargetIP) {
			continue
		}

		err = cli.Reply(pkt, hwaddr, pkt.TargetIP)

		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR cli.Reply: %s\n", err)
			continue
		}
	}
}
