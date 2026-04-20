package netns

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"

	"devup/internal/logging"
)

const (
	nsPrefix = "devup-"
	// Private subnet for veth pairs. Each namespace gets a /30 from 10.200.X.0.
	// Host side: 10.200.X.1, Guest side: 10.200.X.2
	subnetBase = "10.200"
)

// Monotonically increasing counter for unique subnet allocation within this
// agent process lifetime. Wraps at 254 which is fine for dev workloads.
var subnetCounter uint32

func nextSubnetOctet() int {
	n := atomic.AddUint32(&subnetCounter, 1)
	return int((n % 254) + 1)
}

// Create sets up a named network namespace with a veth pair, IP addresses,
// routing, and NAT so the isolated process can reach the internet.
//
// The namespace is named nsName (should start with "devup-").
// The host-side veth is named "v-<suffix>" (max 15 chars for IFNAMSIZ).
func Create(nsName string) error {
	// 1. Create the namespace
	if out, err := run("ip", "netns", "add", nsName); err != nil {
		return fmt.Errorf("ip netns add %s: %w\n%s", nsName, err, out)
	}

	octet := nextSubnetOctet()
	hostIP := fmt.Sprintf("%s.%d.1", subnetBase, octet)
	guestIP := fmt.Sprintf("%s.%d.2", subnetBase, octet)
	subnet := fmt.Sprintf("%s.%d.0/30", subnetBase, octet)

	// veth names: max 15 chars. Use "v-" + last 13 chars of nsName.
	suffix := nsName
	if len(suffix) > 13 {
		suffix = suffix[len(suffix)-13:]
	}
	hostVeth := "v-" + suffix
	guestVeth := "eth0"

	// 2. Create veth pair
	if out, err := run("ip", "link", "add", hostVeth, "type", "veth", "peer", "name", guestVeth); err != nil {
		exec.Command("ip", "netns", "del", nsName).Run()
		return fmt.Errorf("create veth: %w\n%s", err, out)
	}

	// 3. Move guest side into namespace
	if out, err := run("ip", "link", "set", guestVeth, "netns", nsName); err != nil {
		exec.Command("ip", "link", "del", hostVeth).Run()
		exec.Command("ip", "netns", "del", nsName).Run()
		return fmt.Errorf("move veth to ns: %w\n%s", err, out)
	}

	// 4. Configure host side
	run("ip", "addr", "add", hostIP+"/30", "dev", hostVeth)
	run("ip", "link", "set", hostVeth, "up")

	// 5. Configure guest side (inside namespace)
	run("ip", "netns", "exec", nsName, "ip", "addr", "add", guestIP+"/30", "dev", "eth0")
	run("ip", "netns", "exec", nsName, "ip", "link", "set", "eth0", "up")
	run("ip", "netns", "exec", nsName, "ip", "link", "set", "lo", "up")
	run("ip", "netns", "exec", nsName, "ip", "route", "add", "default", "via", hostIP)

	// 6. Enable forwarding and NAT so the namespace can reach the internet
	os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644)
	run("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", subnet, "!", "-o", hostVeth, "-j", "MASQUERADE")
	run("iptables", "-A", "FORWARD", "-i", hostVeth, "-j", "ACCEPT")
	run("iptables", "-A", "FORWARD", "-o", hostVeth, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")

	logging.Info("netns created", "ns", nsName, "host_ip", hostIP, "guest_ip", guestIP)
	return nil
}

// Destroy tears down a network namespace and its associated veth pair.
// The kernel automatically cleans up the veth when the namespace is deleted.
func Destroy(nsName string) error {
	if nsName == "" {
		return nil
	}

	// Remove the veth from host side (if it still exists)
	suffix := nsName
	if len(suffix) > 13 {
		suffix = suffix[len(suffix)-13:]
	}
	hostVeth := "v-" + suffix
	exec.Command("ip", "link", "del", hostVeth).Run()

	// Delete the namespace (also kills any remaining processes inside it)
	if out, err := run("ip", "netns", "del", nsName); err != nil {
		return fmt.Errorf("ip netns del %s: %w\n%s", nsName, err, out)
	}

	// Best-effort: clean up iptables rules referencing this namespace's subnet
	cleanupIptables(hostVeth)

	logging.Info("netns destroyed", "ns", nsName)
	return nil
}

// Reconcile lists all network namespaces with the devup- prefix and destroys
// any whose jobID is not in activeJobIDs.
func Reconcile(activeJobIDs map[string]bool) {
	out, err := exec.Command("ip", "netns", "list").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// ip netns list output: "name" or "name (id: N)"
		name := strings.Fields(line)[0]
		if !strings.HasPrefix(name, nsPrefix) {
			continue
		}
		// Extract jobID: "devup-<jobID>" or "devup-run-<requestID>"
		jobID := strings.TrimPrefix(name, nsPrefix)
		jobID = strings.TrimPrefix(jobID, "run-") // for ephemeral run namespaces
		if activeJobIDs[jobID] {
			continue
		}
		if err := Destroy(name); err != nil {
			logging.Error("netns reconcile: destroy failed", "ns", name, "err", err)
		} else {
			logging.Info("netns reconcile: pruned stale namespace", "ns", name)
		}
	}
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

func cleanupIptables(hostVeth string) {
	// List and delete any rules referencing this veth
	for _, table := range []string{"nat", "filter"} {
		chain := "POSTROUTING"
		if table == "filter" {
			chain = "FORWARD"
		}
		out, err := exec.Command("iptables", "-t", table, "-S", chain).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, hostVeth) {
				continue
			}
			// Convert -A to -D for deletion
			rule := strings.Replace(line, "-A ", "-D ", 1)
			args := strings.Fields(rule)
			if len(args) > 1 {
				cmd := append([]string{"-t", table}, args...)
				exec.Command("iptables", cmd...).Run()
			}
		}
	}
}
