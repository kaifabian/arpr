arper
=====

(**ARP** **e**vil **r**esponder)

The `arper` utility is a configurable ARP responder.
Kudos to [mdlayher](https://github.com/mdlayher) for his awesome [arp](https://github.com/mdlayher/arp) package!
(He did all the heavy lifting.)

Usage
-----

`arper <-i interface> [-e ethernet-address] ip_net1 [ip_net2 ...]`

| Argument | default value | usage |
| -------- | ------------- | ----- |
| -i | -   | name of the network interface to listen (and respond) on |
| -e | (eth addr of -i) | ethernet address to respond with |
