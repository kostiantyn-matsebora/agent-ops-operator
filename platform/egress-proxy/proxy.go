package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The proxy answers every redirected connection in the pod.
//
// It has exactly two behaviours, and which one applies is decided by the
// destination the caller intended — recovered from the connection itself, not
// from anything the caller said:
//
//	a bound MCP endpoint  ->  parse, enforce, forward what is granted
//	anything else         ->  copy bytes, both ways, until one side closes
//
// The second case is the common one and must stay boring. The agent's LLM API
// calls, its git traffic and its package fetches all pass through here, and any
// of them breaking is a broken agent.

type proxyConfig struct {
	listenPort int
	// endpoints resolves what a redirected connection is actually carrying.
	// See endpoints.go — a name never matches the address the kernel recorded,
	// and getting that wrong degrades enforcement to pass-through silently.
	endpoints *endpoints
}

func runProxy(argv []string) error {
	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	if err := fs.Parse(argv); err != nil {
		return err
	}
	port, err := strconv.Atoi(envOr("LISTEN_PORT", "15001"))
	if err != nil {
		return fmt.Errorf("LISTEN_PORT: %w", err)
	}
	cfg := proxyConfig{
		listenPort: port,
		endpoints:  newEndpoints(os.Getenv("MCP_ENDPOINTS"), os.Getenv("CONTROL_URL")),
	}
	stop := make(chan struct{})
	defer close(stop)
	go cfg.endpoints.refreshLoop(30*time.Second, stop)
	return serve(cfg)
}

func serve(cfg proxyConfig) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.listenPort))
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Printf("mediating egress on :%d", cfg.listenPort)

	state := newPolicy()
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go handle(c, cfg, state)
	}
}

// handle serves one redirected connection.
func handle(client net.Conn, cfg proxyConfig, state *policy) {
	defer client.Close()

	dst, err := originalDestination(client)
	if err != nil {
		// Without the intended destination there is nothing to forward TO. That
		// is a closed door, never a guess: guessing here would send the agent's
		// traffic somewhere it did not ask for.
		log.Printf("no original destination, dropping: %v", err)
		return
	}
	upstream, err := net.Dial("tcp", dst)
	if err != nil {
		log.Printf("dial %s: %v", dst, err)
		return
	}
	defer upstream.Close()

	// Buffered ONCE, here, and handed to whichever path serves the connection.
	// The Host is read without consuming it, so a connection that turns out to
	// be neither MCP nor the work contract is still forwarded byte for byte.
	br := bufio.NewReader(client)
	route(br, client, upstream, dst, cfg, state)
}

// route picks the behaviour for one connection.
func route(br *bufio.Reader, client io.Writer, upstream net.Conn, dst string,
	cfg proxyConfig, state *policy) {

	key, isMCP, isControl := cfg.endpoints.classifyBy(dst, peekHost(br))
	switch {
	case isMCP:
		serveMCP(br, client, upstream, key, state)
	case isControl:
		// The work contract. Forwarded verbatim, and read on the way past for
		// the access decision it carries.
		serveControl(br, client, upstream, state)
	default:
		pipeBuffered(br, client, upstream)
	}
}

// peekHost reads the Host header WITHOUT consuming it, and WITHOUT waiting for
// bytes that may never come.
//
// bufio.Peek(n) blocks until it has n bytes or the connection errors. Asking
// for a fixed window therefore stalls every request smaller than that window —
// which is every HTTP request — until the client gives up. That stalls the work
// contract too, so the policy is never learned and everything is then refused
// against an empty allowlist. It reads as an agent whose tools all stopped
// working.
//
// So: block for ONE byte, then take only what that read actually delivered. A
// client writes its request head in a single segment, so the Host is there. If
// it is not, the connection falls back to address classification rather than
// waiting for it.
func peekHost(br *bufio.Reader) string {
	if _, err := br.Peek(1); err != nil {
		return ""
	}
	buf, _ := br.Peek(br.Buffered())
	if len(buf) == 0 {
		return ""
	}
	// Stop at the end of the header block, so a body containing something that
	// looks like a header cannot supply the Host.
	if i := bytes.Index(buf, []byte("\r\n\r\n")); i >= 0 {
		buf = buf[:i]
	}
	for _, line := range bytes.Split(buf, []byte("\r\n")) {
		const h = "host:"
		if len(line) > len(h) && strings.EqualFold(string(line[:len(h)]), h) {
			return strings.TrimSpace(string(line[len(h):]))
		}
	}
	return ""
}

// pipeBuffered copies a connection through, starting from whatever the peek
// already buffered.
func pipeBuffered(br *bufio.Reader, client io.Writer, upstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, br)
		if cw, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(client, upstream)
		if cw, ok := client.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	wg.Wait()
}

// pipe copies both directions until either side closes, streaming rather than
// buffering. MCP itself uses long-lived event streams, and so does a `git
// clone` of any size — a proxy that buffers turns both into a stall.
func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); copyThenCloseWrite(b, a) }()
	go func() { defer wg.Done(); copyThenCloseWrite(a, b) }()
	wg.Wait()
}

// copyThenCloseWrite propagates a half-close, so a protocol that signals "I am
// done sending" by closing its write side keeps working through the proxy.
func copyThenCloseWrite(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// originalDestination recovers where a redirected connection was headed.
//
// The redirect rewrote the destination to this process, so the socket's peer
// address is useless — the kernel keeps the original and this is how it is
// asked for it.
func originalDestination(c net.Conn) (string, error) {
	tcp, ok := c.(*net.TCPConn)
	if !ok {
		return "", errors.New("not a TCP connection")
	}
	f, err := tcp.File()
	if err != nil {
		return "", err
	}
	defer f.Close()
	fd := int(f.Fd())

	dst, err := origDstV4(fd)
	if err != nil {
		dst, err = origDstV6(fd)
		if err != nil {
			return "", err
		}
	}
	if isSelfConnection(dst, c.LocalAddr().String()) {
		// SO_ORIGINAL_DST is not guaranteed to error on a connection nf_conntrack
		// never rewrote — on some kernels it returns the connection's own,
		// untouched destination instead. A caller that dials the proxy's own
		// port directly (bypassing the iptables REDIRECT that makes every
		// legitimately-mediated connection's original destination differ from
		// this address) would otherwise have that self-referential address
		// dialed right back into this same listener: Accept hands the new
		// connection to another handle(), which resolves the same self-address
		// and dials again, without bound, until the process runs out of OS
		// threads. Genuinely redirected traffic never triggers this: REDIRECT
		// preserves the real upstream in conntrack, which is never this
		// listener's own address.
		return "", fmt.Errorf("original destination %s is this proxy's own address, refusing to self-connect", dst)
	}
	return dst, nil
}

// isSelfConnection reports whether a resolved original destination is this
// listener's own local address — the signature of the getsockopt fallback
// above, never a legitimately redirected connection's real destination.
func isSelfConnection(dst, local string) bool {
	return dst == local
}

const (
	soOriginalDst = 80
	// IPv6 is not an optional extra. A v4-only proxy on a dual-stack cluster
	// drops every v6 connection the redirect handed it, which reads as a broken
	// network rather than a missing feature.
	ip6tOriginalDst = 80
)

func origDstV4(fd int) (string, error) {
	a, err := syscall.GetsockoptIPv6Mreq(fd, syscall.IPPROTO_IP, soOriginalDst)
	if err != nil {
		return "", err
	}
	ip := net.IPv4(a.Multiaddr[4], a.Multiaddr[5], a.Multiaddr[6], a.Multiaddr[7])
	port := int(a.Multiaddr[2])<<8 | int(a.Multiaddr[3])
	return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}

// origDstV6 reads the same option for the v6 family. The reply is a
// sockaddr_in6, which no typed helper in syscall returns, so it is read as raw
// bytes and decoded here.
func origDstV6(fd int) (string, error) {
	raw, err := getsockoptBytes(fd, syscall.IPPROTO_IPV6, ip6tOriginalDst, 28)
	if err != nil {
		return "", err
	}
	// sockaddr_in6: family(2) port(2, network order) flowinfo(4) addr(16)
	port := int(raw[2])<<8 | int(raw[3])
	ip := net.IP(raw[8:24])
	return net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}

// hostPortOf reduces a URL to the host:port a connection to it arrives with.
func hostPortOf(raw string) string {
	rest := raw
	scheme := ""
	if i := strings.Index(rest, "://"); i >= 0 {
		scheme, rest = rest[:i], rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return ""
	}
	if strings.Contains(rest, ":") {
		return rest
	}
	if scheme == "https" {
		return rest + ":443"
	}
	return rest + ":80"
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
