package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog/log"

	"github.com/qiangli/aperio/internal/trace"
)

// Mode determines whether the proxy records or replays.
type Mode int

const (
	ModeRecord Mode = iota
	ModeReplay
)

// Proxy wraps a goproxy server with recording/replay capabilities.
type Proxy struct {
	server   *goproxy.ProxyHttpServer
	listener net.Listener
	mode     Mode
	targets  []string // if non-empty, only intercept these hosts

	mu    sync.Mutex
	spans []*trace.Span
}

// Options configures the proxy.
type Options struct {
	Mode    Mode
	Port    int
	Targets []string // LLM API hosts to intercept (empty = all)

	// For replay mode
	RecordedTrace *trace.Trace
	Strict        bool
	MatchStrategy string
	CassettePath  string // Load replay data from YAML cassette file
	Normalize     bool   // Normalize responses before comparison
}

// New creates a new proxy instance.
func New(opts Options) (*Proxy, error) {
	server := goproxy.NewProxyHttpServer()
	server.Verbose = false

	p := &Proxy{
		server:  server,
		mode:    opts.Mode,
		targets: opts.Targets,
	}

	// Set up MITM for HTTPS CONNECT requests
	server.OnRequest().HandleConnect(goproxy.AlwaysMitm)

	switch opts.Mode {
	case ModeRecord:
		setupRecorder(p, server, opts.Targets)
	case ModeReplay:
		setupReplayer(p, server, opts)
	}

	// Start listener
	addr := fmt.Sprintf(":%d", opts.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	p.listener = listener

	return p, nil
}

// Addr returns the listener address (useful when port=0 for random port).
func (p *Proxy) Addr() string {
	return p.listener.Addr().String()
}

// Port returns the port the proxy is listening on.
func (p *Proxy) Port() int {
	return p.listener.Addr().(*net.TCPAddr).Port
}

// Serve starts serving proxy requests. Blocks until the listener is closed.
func (p *Proxy) Serve() error {
	log.Info().Str("addr", p.Addr()).Msg("proxy listening")
	return http.Serve(p.listener, p.server)
}

// Close shuts down the proxy.
func (p *Proxy) Close() error {
	return p.listener.Close()
}

// Spans returns the collected spans (for record mode).
func (p *Proxy) Spans() []*trace.Span {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]*trace.Span, len(p.spans))
	copy(result, p.spans)
	return result
}

func (p *Proxy) addSpan(s *trace.Span) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.spans = append(p.spans, s)
}

// shouldIntercept checks if a request host matches the target filter.
func (p *Proxy) shouldIntercept(host string) bool {
	if len(p.targets) == 0 {
		return true
	}
	for _, target := range p.targets {
		if host == target {
			return true
		}
	}
	return false
}

// SetCA sets a custom CA certificate for MITM.
func SetCA(caCert *x509.Certificate, caKey interface{}) {
	goproxy.GoproxyCa = tls.Certificate{
		Certificate: [][]byte{caCert.Raw},
		PrivateKey:  caKey,
	}
}
