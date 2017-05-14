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
	ifname    = flag.String("i", "", "Network interface")
	ethaddr   = flag.String("e", "", "Ethernet address")
	gratArp   = flag.Bool("g", false, "Send gratuitous ARP")
	gratInt   = flag.Int("G", 60, "Interval to send gratuitous ARP in [seconds]")
	gratMax   = flag.Int("M", 1024, "Maximum number of IP addresses for gratuitous ARP (performance implications!)")
	inclNet   = flag.Bool("N", false, "Include network base address")
	inclBcast = flag.Bool("B", false, "Include network broadcast address")

	myNets     = []net.IPNet{}
	netExcepts = []net.IPNet{}
	args       = []string{}
)

func netBcast(ipNet net.IPNet) net.IP {
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)
	ones, bits := ipNet.Mask.Size()

	pos := len(ip) - 1
	for zeros := int(bits - ones); zeros > 0 ; zeros -= 8 {
		ip[pos] |= ((1 << uint(zeros)) - 1) & 0xFF;
		pos--
	}

	return ip
}

func netAddr(ipNet net.IPNet) net.IP {
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)
	ones, bits := ipNet.Mask.Size()

	pos := len(ip) - 1
	for zeros := int(bits - ones); zeros > 0 ; zeros -= 8 {
		ip[pos] &= ^(((1 << uint(zeros)) - 1) & 0xFF);
		pos--
	}

	return ip
}

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

func incrementIP(iip net.IP) net.IP {
	oip := make(net.IP, len(iip))
	copy(oip, iip)
	for i := int(len(oip)) - 1; i >= 0; i-- {
		if oip[i] >= 255 {
			oip[i] = 0
			continue
		} else {
			oip[i]++
			break
		}
	}
	return oip
}

func allIps(ipNet net.IPNet, ch chan net.IP) {
	ones, bits := ipNet.Mask.Size() // IPMask.Size assumes canonical netmask -- as do we

	if bits == 0 {
		// Non-canonical IP
		return
	}

	numIps := 1 << uint(bits-ones)
	current := net.IP(ipNet.IP)

	for i := 0; i < numIps; i++ {
		ch <- current
		current = incrementIP(current)
	}
}

func allIpsInNets(ipNets []net.IPNet) <-chan net.IP {
	ch := make(chan net.IP)
	go func() {
		for _, ipNet := range ipNets {
			allIps(ipNet, ch)
		}
	}()
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
		exclude := false

		if strings.HasPrefix(arg, "~") {
			exclude = true
			arg = strings.TrimPrefix(arg, "~")
		}

		if !strings.Contains(arg, "/") {
			arg = fmt.Sprintf("%s/32", arg)
		}

		_, ipnet, err := net.ParseCIDR(arg)

		if err != nil {
			fmt.Println(err)
			return
		}

		if exclude {
			netExcepts = append(netExcepts, *ipnet)
		} else {
			ones, bits := ipnet.Mask.Size()
			zeros := uint(bits - ones)

			fmt.Fprintf(os.Stderr, "INFO Listening for %s\n", ipnet.String())
			myNets = append(myNets, *ipnet)

			if zeros >= 2 {
				if !*inclNet {
					exclNet := &net.IPNet{netAddr(*ipnet), net.CIDRMask(bits, bits)}
					netExcepts = append(netExcepts, *exclNet)
					fmt.Fprintf(os.Stderr, "INFO Ignoring %s\n", exclNet.String())
				}
				if !*inclBcast {
					exclNet := &net.IPNet{netBcast(*ipnet), net.CIDRMask(bits, bits)}
					netExcepts = append(netExcepts, *exclNet)
					fmt.Fprintf(os.Stderr, "INFO Ignoring %s\n", exclNet.String())
				}
			}
		}
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
		numAddr += 1 << uint(bits-ones)
	}

	if *gratArp {
		if numAddr > *gratMax {
			usageError("Too many IP addresses for gratuitous ARP -- either decrease number of IP addresses or increase -M!")
			return
		}

		sleepInt := time.Duration(*gratInt) * time.Second

		for ip := range allIpsInNets(myNets) {
			if !netsContain(netExcepts, ip) {
				fmt.Fprintf(os.Stderr, "DEBUG starting gratarp: %s", ip)
				go gratuitousArp(cli, ip, hwaddr, sleepInt)
			}
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

		// Ignore excluded IP addresses
		if netsContain(netExcepts, pkt.TargetIP) {
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
