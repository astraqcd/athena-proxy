package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	GatewayPort = 443

	DefaultDialTimeout      = 10 * time.Second
	DefaultHandshakeTimeout = 15 * time.Second

	bufferSize = 32 * 1024
)

type Config struct {
	Port             int
	DialTimeout      time.Duration
	HandshakeTimeout time.Duration
	RootCAs          *x509.CertPool
	Dial             func(ctx context.Context, addr string) (net.Conn, error)
	Errorf           func(format string, args ...any)
}

func (c Config) withDefaults() Config {
	if c.Port == 0 {
		c.Port = GatewayPort
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = DefaultDialTimeout
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if c.Dial == nil {
		dialer := &net.Dialer{Timeout: c.DialTimeout}
		c.Dial = func(ctx context.Context, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", addr)
		}
	}
	if c.Errorf == nil {
		c.Errorf = func(string, ...any) {}
	}
	return c
}

type Tunnel struct {
	hostname string
	label    string
	config   Config
}

func NewTunnel(hostname, label string, config Config) *Tunnel {
	return &Tunnel{
		hostname: hostname,
		label:    label,
		config:   config.withDefaults(),
	}
}

func (t *Tunnel) Hostname() string {
	return t.hostname
}

type halfCloser interface {
	io.ReadWriter
	CloseWrite() error
}

func (t *Tunnel) Handle(local net.Conn) {
	defer local.Close()

	upstream, err := t.dial()
	if err != nil {
		t.config.Errorf("%s: %v", t.label, err)
		return
	}
	defer upstream.Close()

	localHalf, ok := local.(halfCloser)
	if !ok {
		t.config.Errorf("%s: local connection does not support half-close", t.label)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go copyHalf(&wg, upstream, localHalf)
	go copyHalf(&wg, localHalf, upstream)
	wg.Wait()
}

func (t *Tunnel) dial() (*tls.Conn, error) {
	addr := net.JoinHostPort(t.hostname, strconv.Itoa(t.config.Port))

	ctx, cancel := context.WithTimeout(context.Background(), t.config.DialTimeout+t.config.HandshakeTimeout)
	defer cancel()

	raw, err := t.config.Dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the gateway at %s: %w", addr, err)
	}

	conn := tls.Client(raw, &tls.Config{
		ServerName: t.hostname,
		RootCAs:    t.config.RootCAs,
		MinVersion: tls.VersionTLS12,
	})

	if err := raw.SetDeadline(time.Now().Add(t.config.HandshakeTimeout)); err != nil {
		raw.Close()
		return nil, fmt.Errorf("cannot set a handshake deadline: %w", err)
	}
	if err := conn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("TLS handshake with %s failed: %w", addr, err)
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cannot clear the handshake deadline: %w", err)
	}

	return conn, nil
}

var buffers = sync.Pool{
	New: func() any {
		buf := make([]byte, bufferSize)
		return &buf
	},
}

func copyHalf(wg *sync.WaitGroup, dst, src halfCloser) {
	defer wg.Done()

	buf := buffers.Get().(*[]byte)
	defer buffers.Put(buf)

	_, _ = io.CopyBuffer(writerOnly{dst}, readerOnly{src}, *buf)
	_ = dst.CloseWrite()
}

type writerOnly struct{ io.Writer }

type readerOnly struct{ io.Reader }
