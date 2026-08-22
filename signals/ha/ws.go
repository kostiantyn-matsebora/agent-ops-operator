package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// A minimal RFC 6455 client, hand-written for the same reason signal-k8s-events
// hand-writes its Kubernetes watch: every adapter in this repo is a module with
// NO dependencies outside it, and a WebSocket client is a protocol, not a
// platform. What is implemented is exactly what the Home Assistant WebSocket API
// uses — text frames, fragmentation, ping/pong, close — and nothing else:
// no extensions, no compression, no server side.
//
// Two rules of the protocol are easy to get wrong and are both failures the
// server closes the connection over:
//
//   - every client frame MUST be masked, with a fresh 32-bit key;
//   - control frames (ping, pong, close) may arrive INTERLEAVED between the
//     fragments of a data message, so they are answered where they are read
//     rather than surfaced to the caller.

const (
	// wsMaxMessage bounds one reassembled message. Home Assistant's largest
	// answer here is a system_log listing; a megabyte is far past it and keeps
	// a hostile or broken peer from growing this process without bound.
	wsMaxMessage = 8 << 20
	// wsGUID is the RFC 6455 handshake constant.
	wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

// opcodes. Only the ones this client can send or receive are named.
const (
	opContinuation byte = 0x0
	opText         byte = 0x1
	opBinary       byte = 0x2
	opClose        byte = 0x8
	opPing         byte = 0x9
	opPong         byte = 0xA
)

// errWSClosed is returned when the peer closed the connection cleanly. Callers
// treat it as "reconnect", not as a failure to report.
var errWSClosed = errors.New("websocket closed by peer")

// wsConn is one client connection.
//
// Reads are single-goroutine by contract (the read loop owns them). Writes are
// mutex-guarded because pongs are written from the read path while the caller
// may be sending a command.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex
}

// wsDial performs the opening handshake against a ws:// or wss:// URL.
//
// The handshake is written over a raw connection rather than through
// http.Client on purpose: the client offers no way to take the connection back
// after a 101, which is the entire point of an upgrade.
func wsDial(ctx context.Context, rawURL string, header http.Header) (*wsConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("websocket url %q: %w", rawURL, err)
	}
	var useTLS bool
	switch u.Scheme {
	case "ws":
	case "wss":
		useTLS = true
	default:
		return nil, fmt.Errorf("websocket url %q: scheme must be ws or wss", rawURL)
	}
	host := u.Host
	if u.Port() == "" {
		if useTLS {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	d := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	if useTLS {
		tc := tls.Client(conn, &tls.Config{ServerName: u.Hostname()})
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tc
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	path := u.RequestURI()
	var req strings.Builder
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\n", path)
	fmt.Fprintf(&req, "Host: %s\r\n", u.Host)
	req.WriteString("Upgrade: websocket\r\n")
	req.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&req, "Sec-WebSocket-Key: %s\r\n", key)
	req.WriteString("Sec-WebSocket-Version: 13\r\n")
	for k, vs := range header {
		for _, v := range vs {
			fmt.Fprintf(&req, "%s: %s\r\n", k, v)
		}
	}
	req.WriteString("\r\n")

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	}
	if _, err := io.WriteString(conn, req.String()); err != nil {
		conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), acceptKey(key); got != want {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: peer returned the wrong accept key")
	}
	_ = conn.SetDeadline(time.Time{})
	return &wsConn{conn: conn, br: br}, nil
}

// acceptKey computes the server's expected Sec-WebSocket-Accept value.
func acceptKey(clientKey string) string {
	h := sha1.Sum([]byte(clientKey + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// WriteJSON sends one JSON value as a masked text frame.
func (c *wsConn) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeFrame(opText, b)
}

// writeFrame writes one unfragmented, masked frame.
func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	var head [14]byte
	head[0] = 0x80 | opcode // FIN
	n := 2
	switch {
	case len(payload) < 126:
		head[1] = byte(len(payload))
	case len(payload) <= 0xFFFF:
		head[1] = 126
		binary.BigEndian.PutUint16(head[2:4], uint16(len(payload)))
		n = 4
	default:
		head[1] = 127
		binary.BigEndian.PutUint64(head[2:10], uint64(len(payload)))
		n = 10
	}
	head[1] |= 0x80 // MASK: mandatory on every client frame
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	copy(head[n:n+4], mask[:])
	n += 4

	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if _, err := c.conn.Write(head[:n]); err != nil {
		return err
	}
	if len(masked) == 0 {
		return nil
	}
	_, err := c.conn.Write(masked)
	return err
}

// ReadMessage returns the next complete data message, answering control frames
// on the way. deadline bounds the wait for the next FRAME, not the message:
// Home Assistant is silent between events, so callers pass a generous one and
// use it as their liveness check.
func (c *wsConn) ReadMessage(deadline time.Time) ([]byte, error) {
	var (
		buf    []byte
		opcode byte
	)
	for {
		_ = c.conn.SetReadDeadline(deadline)
		fin, op, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch op {
		case opPing:
			// Answered here rather than surfaced: a control frame may sit
			// BETWEEN the fragments of a data message, so the caller must
			// never see it.
			if err := c.writeFrame(opPong, payload); err != nil {
				return nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			_ = c.writeFrame(opClose, payload)
			return nil, errWSClosed
		case opContinuation:
			if opcode == 0 {
				return nil, errors.New("websocket: continuation frame with nothing to continue")
			}
		case opText, opBinary:
			if opcode != 0 {
				return nil, errors.New("websocket: new data frame inside a fragmented message")
			}
			opcode = op
		default:
			return nil, fmt.Errorf("websocket: unsupported opcode 0x%x", op)
		}
		if len(buf)+len(payload) > wsMaxMessage {
			return nil, fmt.Errorf("websocket: message exceeds %d bytes", wsMaxMessage)
		}
		buf = append(buf, payload...)
		if fin {
			return buf, nil
		}
	}
}

// readFrame reads one frame header and its payload.
func (c *wsConn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var head [2]byte
	if _, err = io.ReadFull(c.br, head[:]); err != nil {
		return false, 0, nil, err
	}
	fin = head[0]&0x80 != 0
	if head[0]&0x70 != 0 {
		// No extension was negotiated, so a reserved bit means the peer is
		// speaking something this client did not agree to.
		return false, 0, nil, errors.New("websocket: reserved bits set")
	}
	opcode = head[0] & 0x0F
	masked := head[1]&0x80 != 0
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > wsMaxMessage {
		return false, 0, nil, fmt.Errorf("websocket: frame of %d bytes exceeds the limit", length)
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return false, 0, nil, err
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return fin, opcode, payload, nil
}

// Close sends a close frame and drops the connection. Errors are ignored: this
// runs on a path where the connection is being abandoned anyway.
func (c *wsConn) Close() error {
	var frame [2]byte
	binary.BigEndian.PutUint16(frame[:], 1000) // normal closure
	_ = c.writeFrame(opClose, frame[:])
	return c.conn.Close()
}
