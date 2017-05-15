package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/mdlayher/arp"
)

var (
	arperPath = flag.String("a", "arper", "path to arper binary")
	arperIf   = flag.String("i", "", "arper interface")
	cliIf     = flag.String("c", "", "client test interface")
	timeOut   = flag.Uint("t", 100, "timeout in milliseconds")
)

type testCase struct {
	Name string
	Arg  []string

	ExpectEth net.HardwareAddr

	ArpSuccess []net.IP
	ArpFail    []net.IP
}

type testRun struct {
	TestCase *testCase
	Cmd      *exec.Cmd
	Fails    uint
	Tests    uint
}

func spawnArper(customArg ...string) *exec.Cmd {
	arg := []string{
		"-i",
		*arperIf,
	}

	for _, v := range customArg {
		arg = append(arg, v)
	}

	cmd := exec.Command(*arperPath, arg...)
	cmd.Start()
	return cmd
}

func parseMAC(mac string) net.HardwareAddr {
	hwaddr, err := net.ParseMAC(mac)

	if err != nil {
		return nil
	}

	return hwaddr
}

func runTestCase(testCase *testCase, ifi *net.Interface) *testRun {
	run := &testRun{testCase, nil, 0, 0}
	run.Cmd = spawnArper(testCase.Arg...)

	time.Sleep(1 * time.Second)

	cli, _ := arp.Dial(ifi)

	expectEth := testCase.ExpectEth
	if expectEth == nil {
		expectEth = ifi.HardwareAddr
	}

	fmt.Fprintf(os.Stderr, "\t%s\n", testCase.Name)

	for _, v := range testCase.ArpSuccess {
		run.Tests++

		cli.SetDeadline(time.Now().Add(time.Duration(*timeOut) * time.Millisecond))
		resEth, err := cli.Resolve(v)

		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL\t%s: resolving %s error: %s\n", testCase.Name, v.String(), err)
			run.Fails++
			continue
		}

		if !bytes.Equal(resEth, expectEth) {
			fmt.Fprintf(os.Stderr, "FAIL\t%s: resolved %s: %s != %s\n", testCase.Name, v.String(), resEth.String(), expectEth.String())
			run.Fails++
			continue
		}

		fmt.Fprintf(os.Stderr, "OK\t%s: resolved %s: %s\n", testCase.Name, v.String(), resEth.String())
	}

	for _, v := range testCase.ArpFail {
		run.Tests++

		cli.SetDeadline(time.Now().Add(time.Duration(*timeOut) * time.Millisecond))
		resEth, err := cli.Resolve(v)

		if err == nil {
			if bytes.Equal(resEth, expectEth) {
				fmt.Fprintf(os.Stderr, "FAIL\t%s: resolved %s successfully: %s\n", testCase.Name, v.String(), resEth.String())
				run.Fails++
				continue
			}
		}

		fmt.Fprintf(os.Stderr, "OK\t%s: unresolved %s\n", testCase.Name, v.String())
	}

	fmt.Fprintf(os.Stderr, "\n")

	run.Cmd.Process.Kill()

	return run
}

func main() {
	flag.Parse()

	ifi, err := net.InterfaceByName(*cliIf)

	if err != nil {
		fmt.Fprintf(os.Stdout, "FAIL HARD cannot open interface %s: %s\n", *cliIf, err)
		os.Exit(1)
	}

	afi, err := net.InterfaceByName(*arperIf)

	if err != nil {
		fmt.Fprintf(os.Stdout, "FAIL HARD cannot open interface %s: %s\n", *cliIf, err)
		os.Exit(1)
	}

	arperEth := afi.HardwareAddr

	testCases := []testCase{
		{
			"single1",
			[]string{"--", "10.0.42.128"},
			arperEth,
			[]net.IP{net.ParseIP("10.0.42.128")},
			[]net.IP{net.ParseIP("10.0.42.127"), net.ParseIP("10.0.42.129"), net.ParseIP("10.0.42.255")},
		},

		{
			"net1",
			[]string{"--", "10.0.42.128/25"},
			arperEth,
			[]net.IP{net.ParseIP("10.0.42.129"), net.ParseIP("10.0.42.170"), net.ParseIP("10.0.42.254")},
			[]net.IP{net.ParseIP("10.0.42.1"), net.ParseIP("10.0.42.128"), net.ParseIP("10.0.42.255")},
		},

		{
			"exclude1",
			[]string{"--", "10.0.42.128/25", "~10.0.42.142"},
			arperEth,
			[]net.IP{net.ParseIP("10.0.42.141"), net.ParseIP("10.0.42.143")},
			[]net.IP{net.ParseIP("10.0.42.142")},
		},

		{
			"netExclude1",
			[]string{"-N", "--", "10.0.42.128/25", "~10.0.42.142"},
			arperEth,
			[]net.IP{net.ParseIP("10.0.42.128")},
			[]net.IP{net.ParseIP("10.0.42.255")},
		},

		{
			"netExclude2",
			[]string{"-B", "--", "10.0.42.128/25", "~10.0.42.142"},
			arperEth,
			[]net.IP{net.ParseIP("10.0.42.255")},
			[]net.IP{net.ParseIP("10.0.42.128")},
		},

		{
			"customMac1",
			[]string{"-e", "00-11-de-ad-be-ef", "--", "10.0.42.42"},
			parseMAC("00-11-de-ad-be-ef"),
			[]net.IP{net.ParseIP("10.0.42.42")},
			[]net.IP{},
		},
	}

	failTests := uint(0)
	overallTests := uint(0)

	failCases := uint(0)
	overallCases := uint(0)

	fmt.Fprintf(os.Stdout, "\tARPER TEST RUNNER\n\n")

	for _, testCase := range testCases {
		testRun := runTestCase(&testCase, ifi)
		failTests += testRun.Fails
		overallTests += testRun.Tests

		if testRun.Fails > 0 {
			failCases++
		}
		overallCases++
	}

	fmt.Fprintf(os.Stdout, "\tOVERALL PASSED %d/%d cases (%d/%d tests)\n", (overallCases - failCases), overallCases, (overallTests - failTests), overallTests)

	if failTests > 0 {
		os.Exit(1)
	}

	os.Exit(0)
}
