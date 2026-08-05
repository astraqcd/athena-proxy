package daemon_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astraqcd/athena-proxy/internal/control"
	"github.com/astraqcd/athena-proxy/internal/daemon"
	"github.com/astraqcd/athena-proxy/internal/proxy"
	"github.com/astraqcd/athena-proxy/internal/tlstest"
)

const (
	hostA = "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz"
	hostB = "n1ubqh2xsyb1q00lwgctna75.challs.ctf-platform.xyz"
	dead  = "dhqw2q86wbz6x5av9dmj3ljw.challs.ctf-platform.xyz"
)

type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type fixture struct {
	daemon *daemon.Daemon
	client *control.Client
	out    *syncBuffer
	err    *syncBuffer
}

func newFixture(t *testing.T, servers map[string]*tlstest.Server) *fixture {
	t.Helper()
	t.Setenv("ATHENA_PROXY_HOME", t.TempDir())
	return start(t, servers)
}

func start(t *testing.T, servers map[string]*tlstest.Server) *fixture {
	t.Helper()

	pool := x509.NewCertPool()
	for _, server := range servers {
		pool.AddCert(server.Leaf)
	}

	f := &fixture{out: &syncBuffer{}, err: &syncBuffer{}}
	f.daemon = daemon.New(daemon.Options{
		Version: "test",
		Out:     f.out,
		Err:     f.err,
		Proxy: proxy.Config{
			RootCAs: pool,
			Dial: func(ctx context.Context, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				server, ok := servers[host]
				if !ok {
					return nil, fmt.Errorf("no instance backs %s", host)
				}
				return server.Dial(ctx, addr)
			},
		},
	})

	if err := f.daemon.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() { f.daemon.Close() })

	f.client = control.NewClient(f.daemon.ControlPort())
	return f
}

func echoServer(t *testing.T, hostname string, handler tlstest.Handler) *tlstest.Server {
	t.Helper()

	server, err := tlstest.NewEcho(handler, hostname)
	if err != nil {
		t.Fatalf("start echo server for %s: %v", hostname, err)
	}
	t.Cleanup(func() { server.Close() })
	return server
}

func exchange(t *testing.T, addr, payload string) string {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.(*net.TCPConn).CloseWrite()

	got, err := io.ReadAll(conn)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read timed out: %v", err)
	}
	return string(got)
}

func TestAddOpensAListenerWithoutARestart(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
	})

	resp, err := f.client.Add(control.AddRequest{Hostname: hostA, Name: "pwn"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if resp.Existing {
		t.Fatal("add reported an existing target on a fresh daemon")
	}
	if !strings.HasPrefix(resp.Target.LocalAddr, "127.0.0.1:") {
		t.Fatalf("local address is %s, want a loopback address", resp.Target.LocalAddr)
	}

	if got := exchange(t, resp.Target.LocalAddr, "ping"); got != "ping" {
		t.Fatalf("round-tripped %q, want %q", got, "ping")
	}

	list, err := f.client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Targets) != 1 || list.Targets[0].Name != "pwn" {
		t.Fatalf("list returned %+v, want one target named pwn", list.Targets)
	}
}

func TestTwoTargetsHoldDistinctAddressesAndDoNotCross(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.DrainThenReply("|A")),
		hostB: echoServer(t, hostB, tlstest.DrainThenReply("|B")),
	})

	first, err := f.client.Add(control.AddRequest{Hostname: hostA, Name: "a"})
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	second, err := f.client.Add(control.AddRequest{Hostname: hostB, Name: "b"})
	if err != nil {
		t.Fatalf("add second: %v", err)
	}

	if first.Target.LocalPort == second.Target.LocalPort {
		t.Fatalf("both targets took local port %d", first.Target.LocalPort)
	}

	if got := exchange(t, first.Target.LocalAddr, "x"); got != "x|A" {
		t.Fatalf("first target answered %q, want %q", got, "x|A")
	}
	if got := exchange(t, second.Target.LocalAddr, "x"); got != "x|B" {
		t.Fatalf("second target answered %q, want %q", got, "x|B")
	}
}

func TestADeadIdentityIsRefusedAndItsListenerSurvives(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
	})

	live, err := f.client.Add(control.AddRequest{Hostname: hostA, Name: "live"})
	if err != nil {
		t.Fatalf("add live: %v", err)
	}
	expired, err := f.client.Add(control.AddRequest{Hostname: dead, Name: "expired"})
	if err != nil {
		t.Fatalf("add expired: %v", err)
	}

	if got := exchange(t, expired.Target.LocalAddr, "ping"); got != "" {
		t.Fatalf("a target with no backing returned %q, want nothing", got)
	}
	if !strings.Contains(f.err.String(), "cannot reach the gateway") {
		t.Fatalf("stderr held %q, want a dial failure", f.err.String())
	}

	if got := exchange(t, expired.Target.LocalAddr, "ping"); got != "" {
		t.Fatalf("the listener stopped accepting after a failed connection: %q", got)
	}
	if got := exchange(t, live.Target.LocalAddr, "ping"); got != "ping" {
		t.Fatalf("the live target answered %q after a failure on another target, want %q", got, "ping")
	}
}

func TestTargetsKeepTheirLocalAddressesAcrossARestart(t *testing.T) {
	t.Setenv("ATHENA_PROXY_HOME", t.TempDir())

	servers := map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
		hostB: echoServer(t, hostB, tlstest.Echo),
	}

	first := start(t, servers)
	a, err := first.client.Add(control.AddRequest{Hostname: hostA, Name: "a"})
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	b, err := first.client.Add(control.AddRequest{Hostname: hostB, Name: "b"})
	if err != nil {
		t.Fatalf("add second: %v", err)
	}
	if err := first.daemon.Close(); err != nil {
		t.Fatalf("close daemon: %v", err)
	}

	second := start(t, servers)
	list, err := second.client.List()
	if err != nil {
		t.Fatalf("list after restart: %v", err)
	}

	restored := map[string]int{}
	for _, target := range list.Targets {
		restored[target.Name] = target.LocalPort
	}
	if restored["a"] != a.Target.LocalPort || restored["b"] != b.Target.LocalPort {
		t.Fatalf("restored ports %v, want a=%d b=%d", restored, a.Target.LocalPort, b.Target.LocalPort)
	}

	if got := exchange(t, a.Target.LocalAddr, "ping"); got != "ping" {
		t.Fatalf("restored target answered %q, want %q", got, "ping")
	}
}

func TestPinnedPortIsHonoured(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	free := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	resp, err := f.client.Add(control.AddRequest{Hostname: hostA, Port: free})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if resp.Target.LocalPort != free {
		t.Fatalf("target took port %d, want the pinned %d", resp.Target.LocalPort, free)
	}
	if resp.Reassigned {
		t.Fatal("add reported a reassignment for a free pinned port")
	}
}

func TestAPinnedPortInUseIsReassigned(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
	})

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer taken.Close()
	port := taken.Addr().(*net.TCPAddr).Port

	resp, err := f.client.Add(control.AddRequest{Hostname: hostA, Port: port})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !resp.Reassigned {
		t.Fatal("add did not report the reassignment")
	}
	if resp.Target.LocalPort == port {
		t.Fatalf("target took port %d, which was already in use", port)
	}
	if resp.Requested != port {
		t.Fatalf("add reported requested port %d, want %d", resp.Requested, port)
	}
}

func TestRemoveClosesTheListener(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
	})

	added, err := f.client.Add(control.AddRequest{Hostname: hostA, Name: "pwn"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := f.client.Remove("pwn"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := net.DialTimeout("tcp", added.Target.LocalAddr, 2*time.Second); err == nil {
		t.Fatalf("%s still accepts connections after remove", added.Target.LocalAddr)
	}

	list, err := f.client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Targets) != 0 {
		t.Fatalf("list returned %+v after remove, want nothing", list.Targets)
	}

	if _, err := f.client.Remove("pwn"); err == nil {
		t.Fatal("removing an unknown selector succeeded")
	}
}

func TestRegisteringTheSameHostnameTwiceReturnsTheSameAddress(t *testing.T) {
	f := newFixture(t, map[string]*tlstest.Server{
		hostA: echoServer(t, hostA, tlstest.Echo),
	})

	first, err := f.client.Add(control.AddRequest{Hostname: hostA})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	second, err := f.client.Add(control.AddRequest{Hostname: hostA, Name: "pwn"})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	if !second.Existing {
		t.Fatal("the second add did not report the target as already registered")
	}
	if second.Target.LocalPort != first.Target.LocalPort {
		t.Fatalf("the second add moved the target to port %d, want %d", second.Target.LocalPort, first.Target.LocalPort)
	}
	if second.Target.Name != "pwn" {
		t.Fatalf("the second add left the name as %q, want pwn", second.Target.Name)
	}
}

func TestManyTargetsAcceptIndependently(t *testing.T) {
	servers := map[string]*tlstest.Server{}
	hostnames := make([]string, 0, 12)
	for i := range 12 {
		hostname := fmt.Sprintf("t%023d.challs.ctf-platform.xyz", i)
		hostnames = append(hostnames, hostname)
		servers[hostname] = echoServer(t, hostname, tlstest.Echo)
	}

	f := newFixture(t, servers)

	addresses := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		resp, err := f.client.Add(control.AddRequest{Hostname: hostname})
		if err != nil {
			t.Fatalf("add %s: %v", hostname, err)
		}
		addresses = append(addresses, resp.Target.LocalAddr)
	}

	done := make(chan string, len(addresses))
	for _, addr := range addresses {
		go func() { done <- exchange(t, addr, "ping") }()
	}
	for range addresses {
		select {
		case got := <-done:
			if got != "ping" {
				t.Errorf("round-tripped %q, want %q", got, "ping")
			}
		case <-time.After(30 * time.Second):
			t.Fatal("a target blocked the others")
		}
	}
}

func TestMutationsRequireAJSONContentType(t *testing.T) {
	f := newFixture(t, nil)

	addr := fmt.Sprintf("http://127.0.0.1:%d%s", f.daemon.ControlPort(), control.PathTargets)
	body := strings.NewReader(`{"hostname":"` + hostA + `"}`)

	resp, err := http.Post(addr, "application/x-www-form-urlencoded", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("form-encoded post returned %s, want 415", resp.Status)
	}
	if list, err := f.client.List(); err != nil || len(list.Targets) != 0 {
		t.Fatalf("the form-encoded post registered a target: %+v (%v)", list, err)
	}
}

func TestRequestsFromAnotherOriginHostAreRefused(t *testing.T) {
	f := newFixture(t, nil)

	url := fmt.Sprintf("http://127.0.0.1:%d%s", f.daemon.ControlPort(), control.PathTargets)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "attacker.example.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a rebound Host returned %s, want 403", resp.Status)
	}
}

func TestTheControlPortBindsLoopbackOnly(t *testing.T) {
	f := newFixture(t, nil)

	if _, err := f.client.Add(control.AddRequest{Hostname: hostA}); err != nil {
		t.Fatalf("add: %v", err)
	}

	for _, addr := range boundAddresses(t, f) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatalf("split %s: %v", addr, err)
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			t.Errorf("%s is not a loopback address", addr)
		}
	}
}

func boundAddresses(t *testing.T, f *fixture) []string {
	t.Helper()

	list, err := f.client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	addresses := []string{fmt.Sprintf("127.0.0.1:%d", f.daemon.ControlPort())}
	for _, target := range list.Targets {
		addresses = append(addresses, target.LocalAddr)
	}
	return addresses
}

func TestAddRejectsAnUnusableHostname(t *testing.T) {
	f := newFixture(t, nil)

	for _, hostname := range []string{"", "localhost", "S8JS81P52QT5SIBPGDWRJHIX.challs.ctf-platform.xyz", "s8js81p52qt5sibpgdwrjhix.challs.example.com"} {
		if _, err := f.client.Add(control.AddRequest{Hostname: hostname}); err == nil {
			t.Errorf("add(%q) succeeded, want a rejection", hostname)
		}
	}
}
