package tunnel

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRouteIsolationLifecycleInNetNS(t *testing.T) {
	if os.Getenv("AIMILI_NETNS_CHECK") != "1" {
		t.Skip("requires an isolated network namespace")
	}

	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s failed: %v: %s", name, strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	allBefore, err := readProcSysctl("/proc/sys/net/ipv4/conf/all/rp_filter")
	if err != nil {
		t.Fatalf("read rp_filter before setup: %v", err)
	}

	run("ip", "link", "add", "tun0", "type", "dummy")
	defer exec.Command("ip", "link", "del", "tun0").Run()
	run("ip", "link", "set", "tun0", "up")

	if err := setupTunnelInterface("tun0", 0); err != nil {
		t.Fatalf("setupTunnelInterface failed: %v", err)
	}
	if rules := run("ip", "rule", "show"); !strings.Contains(rules, "oif tun0 lookup 100") {
		t.Fatalf("policy rule missing after setup: %s", rules)
	}
	if routes := run("ip", "route", "show", "table", "100"); !strings.Contains(routes, "default dev tun0") {
		t.Fatalf("isolated default route missing after setup: %s", routes)
	}
	if got, _ := readProcSysctl("/proc/sys/net/ipv4/conf/all/rp_filter"); got != "2" {
		t.Fatalf("expected rp_filter 2 after setup, got %s", got)
	}

	if err := teardownTunnelInterface("tun0", 0); err != nil {
		t.Fatalf("teardownTunnelInterface failed: %v", err)
	}
	if rules := run("ip", "rule", "show"); strings.Contains(rules, "oif tun0 lookup 100") {
		t.Fatalf("policy rule remained after teardown: %s", rules)
	}
	if routes := run("ip", "route", "show", "table", "100"); strings.Contains(routes, "default dev tun0") {
		t.Fatalf("isolated default route remained after teardown: %s", routes)
	}
	if got, _ := readProcSysctl("/proc/sys/net/ipv4/conf/all/rp_filter"); got != allBefore {
		t.Fatalf("rp_filter was not restored: before=%s after=%s", allBefore, got)
	}
}
