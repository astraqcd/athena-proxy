package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/astraqcd/athena-proxy/internal/control"
	"github.com/astraqcd/athena-proxy/internal/proxy"
	"github.com/astraqcd/athena-proxy/internal/state"
)

const (
	loopback = "127.0.0.1"

	portBase  = 13370
	portRange = 500
)

type Options struct {
	ControlPort int
	Version     string
	Proxy       proxy.Config
	Out         io.Writer
	Err         io.Writer
}

type Daemon struct {
	opts    Options
	mu      sync.Mutex
	logMu   sync.Mutex
	targets map[string]*target
	control net.Listener
	server  *http.Server
}

type target struct {
	hostname string
	name     string
	port     int
	listener net.Listener
	tunnel   *proxy.Tunnel
}

func (t *target) label() string {
	if t.name != "" {
		return t.name
	}
	return proxy.ShortHostname(t.hostname)
}

func (t *target) view() control.Target {
	return control.Target{
		Hostname:  t.hostname,
		Name:      t.name,
		LocalPort: t.port,
		LocalAddr: net.JoinHostPort(loopback, strconv.Itoa(t.port)),
	}
}

func New(opts Options) *Daemon {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	d := &Daemon{opts: opts, targets: map[string]*target{}}
	if d.opts.Proxy.Errorf == nil {
		d.opts.Proxy.Errorf = d.errorf
	}
	return d
}

func (d *Daemon) Start() error {
	saved, err := state.Load()
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(loopback, strconv.Itoa(d.opts.ControlPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("open control port: %w", err)
	}
	d.control = listener

	d.server = &http.Server{
		Handler:           d.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if err := d.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.errorf("control port: %v", err)
		}
	}()

	d.restore(saved.Targets)

	if err := d.persist(); err != nil {
		return err
	}

	for _, t := range d.list() {
		fmt.Fprintf(d.opts.Out, "%s\t%s\n", t.LocalAddr, labelOf(t))
	}
	return nil
}

func (d *Daemon) ControlPort() int {
	if d.control == nil {
		return 0
	}
	return d.control.Addr().(*net.TCPAddr).Port
}

func (d *Daemon) Close() error {
	d.mu.Lock()
	for _, t := range d.targets {
		t.listener.Close()
	}
	d.targets = map[string]*target{}
	d.mu.Unlock()

	if d.server != nil {
		return d.server.Close()
	}
	return nil
}

func (d *Daemon) restore(saved []state.Target) {
	sort.Slice(saved, func(i, j int) bool { return saved[i].LocalPort < saved[j].LocalPort })

	for _, s := range saved {
		hostname, err := proxy.NormalizeHostname(s.Hostname)
		if err != nil {
			d.errorf("dropping unusable target from the state file: %v", err)
			continue
		}
		t, reassigned, err := d.open(hostname, s.Name, s.LocalPort)
		if err != nil {
			d.errorf("%s: %v", proxy.ShortHostname(hostname), err)
			continue
		}
		if reassigned {
			d.errorf("%s: local port %d was in use, now on %d — re-point your tool", t.label(), s.LocalPort, t.port)
		}
		d.mu.Lock()
		d.targets[hostname] = t
		d.mu.Unlock()
	}
}

func (d *Daemon) Add(req control.AddRequest) (control.AddResponse, error) {
	hostname, err := proxy.NormalizeHostname(req.Hostname)
	if err != nil {
		return control.AddResponse{}, err
	}
	if req.Port != 0 && (req.Port < 1 || req.Port > 65535) {
		return control.AddResponse{}, fmt.Errorf("local port %d is out of range", req.Port)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.targets[hostname]; ok {
		if req.Port != 0 && req.Port != existing.port {
			return control.AddResponse{}, fmt.Errorf(
				"%s is already registered on %s; remove it before pinning another local port",
				existing.label(), existing.view().LocalAddr)
		}
		if req.Name != "" && req.Name != existing.name {
			if err := d.checkName(req.Name, hostname); err != nil {
				return control.AddResponse{}, err
			}
			existing.name = req.Name
			if err := d.save(); err != nil {
				return control.AddResponse{}, err
			}
		}
		return control.AddResponse{Target: existing.view(), Existing: true}, nil
	}

	if req.Name != "" {
		if err := d.checkName(req.Name, hostname); err != nil {
			return control.AddResponse{}, err
		}
	}

	t, reassigned, err := d.open(hostname, req.Name, req.Port)
	if err != nil {
		return control.AddResponse{}, err
	}
	d.targets[hostname] = t

	if err := d.save(); err != nil {
		t.listener.Close()
		delete(d.targets, hostname)
		return control.AddResponse{}, err
	}

	return control.AddResponse{
		Target:     t.view(),
		Reassigned: reassigned,
		Requested:  req.Port,
	}, nil
}

func (d *Daemon) Remove(selector string) (control.Target, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for hostname, t := range d.targets {
		if !t.matches(selector) {
			continue
		}
		view := t.view()
		t.listener.Close()
		delete(d.targets, hostname)
		if err := d.save(); err != nil {
			return control.Target{}, err
		}
		return view, nil
	}
	return control.Target{}, fmt.Errorf("no target matches %q", selector)
}

func (t *target) matches(selector string) bool {
	if selector == t.hostname || (t.name != "" && selector == t.name) {
		return true
	}
	port, err := strconv.Atoi(selector)
	return err == nil && port == t.port
}

func (d *Daemon) List() []control.Target {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.list()
}

func (d *Daemon) list() []control.Target {
	out := make([]control.Target, 0, len(d.targets))
	for _, t := range d.targets {
		out = append(out, t.view())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LocalPort < out[j].LocalPort })
	return out
}

func (d *Daemon) checkName(name, hostname string) error {
	if _, err := strconv.Atoi(name); err == nil {
		return fmt.Errorf("name %q is a number, which would collide with a local port selector", name)
	}
	for _, t := range d.targets {
		if t.name == name && t.hostname != hostname {
			return fmt.Errorf("name %q is already used by another target", name)
		}
	}
	return nil
}

func (d *Daemon) open(hostname, name string, preferred int) (*target, bool, error) {
	listener, port, reassigned, err := listen(preferred)
	if err != nil {
		return nil, false, err
	}

	t := &target{
		hostname: hostname,
		name:     name,
		port:     port,
		listener: listener,
	}
	t.tunnel = proxy.NewTunnel(hostname, t.label(), d.opts.Proxy)

	go d.accept(t)
	return t, reassigned, nil
}

func (d *Daemon) accept(t *target) {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				d.errorf("%s: stopped accepting on %d: %v", t.label(), t.port, err)
			}
			return
		}
		go t.tunnel.Handle(conn)
	}
}

func listen(preferred int) (net.Listener, int, bool, error) {
	if preferred != 0 {
		listener, err := net.Listen("tcp", net.JoinHostPort(loopback, strconv.Itoa(preferred)))
		if err == nil {
			return listener, preferred, false, nil
		}
	}

	for port := portBase; port < portBase+portRange; port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort(loopback, strconv.Itoa(port)))
		if err == nil {
			return listener, port, preferred != 0, nil
		}
	}
	return nil, 0, false, fmt.Errorf("no free local port between %d and %d", portBase, portBase+portRange-1)
}

func (d *Daemon) persist() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.save()
}

func (d *Daemon) save() error {
	targets := make([]state.Target, 0, len(d.targets))
	for _, t := range d.targets {
		targets = append(targets, state.Target{
			Hostname:  t.hostname,
			Name:      t.name,
			LocalPort: t.port,
		})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].LocalPort < targets[j].LocalPort })

	return state.Save(state.State{ControlPort: d.ControlPort(), Targets: targets})
}

func (d *Daemon) errorf(format string, args ...any) {
	d.logMu.Lock()
	defer d.logMu.Unlock()
	fmt.Fprintf(d.opts.Err, format+"\n", args...)
}

func labelOf(t control.Target) string {
	if t.Name != "" {
		return t.Name
	}
	return proxy.ShortHostname(t.Hostname)
}
