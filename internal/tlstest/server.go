package tlstest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"time"
)

type Handler func(net.Conn)

type Server struct {
	RootCAs  *x509.CertPool
	Leaf     *x509.Certificate
	listener net.Listener
	mu       sync.Mutex
	names    []string
}

func NewEcho(handler Handler, hostnames ...string) (*Server, error) {
	cert, pool, err := selfSigned(hostnames)
	if err != nil {
		return nil, err
	}

	s := &Server{RootCAs: pool, Leaf: cert.Leaf}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			s.mu.Lock()
			s.names = append(s.names, hello.ServerName)
			s.mu.Unlock()
			return nil, nil
		},
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", config)
	if err != nil {
		return nil, err
	}
	s.listener = listener

	go s.serve(handler)
	return s, nil
}

func (s *Server) serve(handler Handler) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go func() {
			defer conn.Close()
			handler(conn)
		}()
	}
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) ServerNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.names...)
}

func (s *Server) Dial(ctx context.Context, _ string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", s.Addr())
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func Echo(conn net.Conn) {
	_, _ = io.Copy(conn, conn)
}

func DrainThenReply(reply string) Handler {
	return func(conn net.Conn) {
		drained, err := io.ReadAll(conn)
		if err != nil {
			return
		}
		_, _ = conn.Write(append(drained, reply...))
	}
}

func selfSigned(hostnames []string) (tls.Certificate, *x509.CertPool, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "athena-proxy test"},
		DNSNames:              hostnames,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool, nil
}
