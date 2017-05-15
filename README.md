arpr [![build status](https://travis-ci.org/kaifabian/arpr.svg?branch=master)](https://travis-ci.org/kaifabian/arpr)
=====

(**ARP** **r**esponder)

The `arpr` utility is a configurable ARP responder.
Kudos to [mdlayher](https://github.com/mdlayher) for his awesome [arp](https://github.com/mdlayher/arp) package!
(He did all the heavy lifting.)


## Installation

(Make sure that you have the Go environment installed and set up...)

`go get github.com/kaifabian/arpr/cmd/arpr` installs the arpr binary into the `$GOPATH/bin` directory.
Of course you can then copy the binary to a more convenient location, e. g. `/usr/local/bin`.


## Usage

`arpr <-i interface> [-e ethernet-address] [-g] [-G send-interval] [-M max-ips] ip_net1 [ip_net2 ...]`

| Argument | default value | usage |
| -------- | ------------- | ----- |
| -i | -   | name of the network interface to listen (and respond) on |
| -e | (eth addr of -i) | ethernet address to respond with |
| -g | false | enable gratuitous ARP |
| -G | 60 | gratuitous ARP send interval (seconds) |
| -M | 1024 | reject gratuitous ARP if more than `-M` IPs (performance implications) |
| -N | false | Do not exclude network base address |
| -B | false | Do not exclude network broadcast address |

`ip_net` can either be a single IP address (e. g. `10.42.13.37`) or a CIDR notation (e. g. `192.168.0.0/16`).
If `ip_net` starts with the character `~`, `ip_net` will be *excluded*.
(Network exclusions precede inclusions!)

At least 1 `ip_net` must be provided.

### Example 1:

`arpr -i eth0 -e 00-11-22-33-44-55 -g -G 30 10.13.37.0/24`

*arpr* will respond to ARP requets to `10.137.0.1 - 10.137.0.254` on *eth0* with MAC address `00-11-22-33-44-55` and send gratuitous ARP packets every 30 seconds.

### Example 2:

`arpr -i em0 -g 10.0.0.0/8`

*arpr* will fail because the user requested gratuitous ARP for more than 1024 IP addresses.

### Example 3:

`arpr -i br-test -N -B 10.0.42.64/26`

*arpr* will respond to ARP requests to `10.0.42.64 - 10.0.42.127` on *br-test* with the device MAC address of *br-test*.

### Example 4:

`arpr -i enp0s1 10.0.0.0/8 ~10.42.0.0/16`

*arpr* will respond to ARP requests to `10.0.0.1 - 10.41.255.255` and `10.43.0.0 - 10.255.255.254` on *enp0s1* with the device MAC address of *enp0s1*.
