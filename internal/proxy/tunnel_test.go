package proxy_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astraqcd/athena-proxy/internal/proxy"
	"github.com/astraqcd/athena-proxy/internal/tlstest"
)

const testHostname = "s8js81p52qt5sibpgdwrjhix.challs.ctf-platform.xyz"

type recorder struct {
	mu       sync.Mutex
	messages []string
}

func (r *recorder) errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.messages...)
}

func localAddr(t *testing.T, tunnel *proxy.Tunnel) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go tunnel.Handle(conn)
		}
	}()

	return listener.Addr().String()
}

func dial(t *testing.T, addr string) *net.TCPConn {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	return conn.(*net.TCPConn)
}

func TestBytesRoundTripWithTheExpectedSNI(t *testing.T) {
	server, err := tlstest.NewEcho(tlstest.Echo, testHostname)
	if err != nil {
		t.Fatalf("start echo server: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	tunnel := proxy.NewTunnel(testHostname, "target", proxy.Config{
		Dial:    server.Dial,
		RootCAs: server.RootCAs,
	})

	conn := dial(t, localAddr(t, tunnel))
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("round-tripped %q, want %q", buf, "ping")
	}

	names := server.ServerNames()
	if len(names) != 1 || names[0] != testHostname {
		t.Fatalf("server observed SNI %v, want [%s]", names, testHostname)
	}
}

func TestHalfClosePropagates(t *testing.T) {
	server, err := tlstest.NewEcho(tlstest.DrainThenReply("|pong"), testHostname)
	if err != nil {
		t.Fatalf("start echo server: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	tunnel := proxy.NewTunnel(testHostname, "target", proxy.Config{
		Dial:    server.Dial,
		RootCAs: server.RootCAs,
	})

	conn := dial(t, localAddr(t, tunnel))
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := conn.CloseWrite(); err != nil {
		t.Fatalf("close write: %v", err)
	}

	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read after half-close: %v", err)
	}
	if string(got) != "ping|pong" {
		t.Fatalf("read %q after half-close, want %q", got, "ping|pong")
	}
}

func TestDialFailureIsReportedAndReachesNothing(t *testing.T) {
	log := &recorder{}
	tunnel := proxy.NewTunnel(testHostname, "target", proxy.Config{
		Dial: func(context.Context, string) (net.Conn, error) {
			return nil, errors.New("no route to host")
		},
		Errorf: log.errorf,
	})

	conn := dial(t, localAddr(t, tunnel))
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("read %q from a target with no backing, want nothing", got)
	}

	messages := log.all()
	if len(messages) != 1 || !strings.Contains(messages[0], "cannot reach the gateway") {
		t.Fatalf("logged %v, want a single dial failure", messages)
	}
}

func TestHandshakeFailureIsReportedSeparately(t *testing.T) {
	plain, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { plain.Close() })

	go func() {
		for {
			conn, err := plain.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("not tls at all"))
			conn.Close()
		}
	}()

	log := &recorder{}
	tunnel := proxy.NewTunnel(testHostname, "target", proxy.Config{
		Dial: func(ctx context.Context, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "tcp", plain.Addr().String())
		},
		Errorf: log.errorf,
	})

	conn := dial(t, localAddr(t, tunnel))
	if _, err := io.ReadAll(conn); err != nil {
		t.Fatalf("read: %v", err)
	}

	messages := log.all()
	if len(messages) != 1 || !strings.Contains(messages[0], "TLS handshake") {
		t.Fatalf("logged %v, want a single handshake failure", messages)
	}
}

func TestConcurrentConnectionsDoNotBlockEachOther(t *testing.T) {
	release := make(chan struct{})
	handler := func(conn net.Conn) {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		if string(buf) == "slow" {
			<-release
		}
		_, _ = conn.Write(buf)
	}

	server, err := tlstest.NewEcho(handler, testHostname)
	if err != nil {
		t.Fatalf("start echo server: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	tunnel := proxy.NewTunnel(testHostname, "target", proxy.Config{
		Dial:    server.Dial,
		RootCAs: server.RootCAs,
	})
	addr := localAddr(t, tunnel)

	slow := dial(t, addr)
	if _, err := slow.Write([]byte("slow")); err != nil {
		t.Fatalf("write: %v", err)
	}

	fast := dial(t, addr)
	if _, err := fast.Write([]byte("fast")); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 4)
	if _, err := io.ReadFull(fast, buf); err != nil {
		t.Fatalf("read from the fast connection while the slow one is held: %v", err)
	}
	if string(buf) != "fast" {
		t.Fatalf("fast connection read %q, want %q", buf, "fast")
	}

	close(release)
	if _, err := io.ReadFull(slow, buf); err != nil {
		t.Fatalf("read from the slow connection: %v", err)
	}
	if string(buf) != "slow" {
		t.Fatalf("slow connection read %q, want %q", buf, "slow")
	}
}
