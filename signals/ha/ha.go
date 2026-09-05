package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// The Home Assistant WebSocket API, which is this adapter's DATA SOURCE.
//
// Why the WebSocket and not the REST log endpoint: `system_log_event` is a
// STRUCTURED record — logger, level, message, source location, occurrence count
// and first-occurrence time — and it is the closest analogue Home Assistant has
// to a Kubernetes Warning event. `GET /api/error_log` returns the raw log FILE,
// so every one of those fields would have to be recovered by regex and the
// cursor would be a byte offset that log rotation invalidates.
//
// The commands beyond the subscription, each earning its place:
//
//	system_log/list              polled every fifteen seconds — the log itself,
//	                             and the dwell re-check's evidence (a record's
//	                             `count` rising means it is still happening)
//	config_entries/get           the config-entry surface, and the log lane's
//	                             health predicate: an entry in setup_error /
//	                             setup_retry is still broken
//	repairs/list_issues          the repair surface, on the same sweep
//	get_states                   the sensor and update surfaces, once a minute
//	config/entity_registry/list  which integration owns a sensor, every fifth
//	                             states read
//	auth/current_user            self-exclusion mechanism 3 — who this
//	                             adapter's token is
//
// A surface that is off issues none of its commands.
//
// Everything here is read-only. This adapter never calls a service, and holding
// a token that could is the ops AGENT's business, not the ingest lane's.

const (
	// haPingEvery keeps the connection warm and, more usefully, notices a dead
	// peer: Home Assistant is silent for hours on a healthy install, so silence
	// alone tells us nothing.
	haPingEvery = 30 * time.Second
	// haReadTimeout must exceed the ping interval by enough that a slow answer
	// is not read as a dead connection.
	haReadTimeout = 90 * time.Second
	// haCommandTimeout bounds one request/response exchange.
	haCommandTimeout = 30 * time.Second
)

// logRecord is one Home Assistant system_log entry, as delivered by the
// `system_log_event` event and by `system_log/list`.
type logRecord struct {
	// Name is the Python logger, e.g. homeassistant.components.zwave_js.
	Name string `json:"name"`
	// Message is a LIST in Home Assistant's own shape, not a string.
	Message []string `json:"message"`
	Level   string   `json:"level"`
	// Source is the heterogeneous [file, lineNumber] pair.
	Source []json.RawMessage `json:"source"`
	// Timestamp and FirstOccurred are epoch seconds with a fractional part.
	Timestamp     float64 `json:"timestamp"`
	FirstOccurred float64 `json:"first_occurred"`
	Exception     string  `json:"exception"`
	// Count is how many times this record has occurred since Home Assistant
	// started. It is what makes a dwell re-check possible: a record whose count
	// has risen during the window is still happening.
	Count int `json:"count"`

	// Surface names which of the instance's health surfaces this record came
	// from, and is EMPTY for a log record. Set by the surface normalisers in
	// surfaces.go and never decoded: a condition of any surface is shaped as a
	// record so that it takes the one consider path — self-exclusion, scope,
	// rules, inhibition, dwell — rather than a second one that drifts.
	Surface string `json:"-"`
	// Integration is a surface record's own integration, where the logger
	// derivation does not apply.
	Integration string `json:"-"`
	// Extra carries a surface record's own payload fields.
	Extra map[string]any `json:"-"`
}

// Text joins the message list into one string.
func (r *logRecord) Text() string { return strings.Join(r.Message, " ") }

// Location renders the source pair as file:line, which together with the logger
// is Home Assistant's own deduplication key — and therefore this adapter's.
func (r *logRecord) Location() string {
	if len(r.Source) == 0 {
		return ""
	}
	var file string
	if err := json.Unmarshal(r.Source[0], &file); err != nil {
		file = strings.Trim(string(r.Source[0]), `"`)
	}
	if len(r.Source) < 2 {
		return file
	}
	var line int
	if err := json.Unmarshal(r.Source[1], &line); err != nil {
		return file
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// Key is the record's identity across occurrences: logger plus source location.
// Deliberately NOT the timestamp or the count — a recurring problem must keep
// one identity so the manager's cooldown can collapse it, exactly as the
// cluster-events adapter keys on the involved object rather than on the Event.
func (r *logRecord) Key() string { return r.Name + "@" + r.Location() }

// At is the record's occurrence time.
func (r *logRecord) At() time.Time { return epoch(r.Timestamp) }

func epoch(f float64) time.Time {
	if f <= 0 {
		return time.Time{}
	}
	sec := int64(f)
	return time.Unix(sec, int64((f-float64(sec))*1e9)).UTC()
}

// configEntry is one integration instance, with the state that makes it a
// health predicate.
type configEntry struct {
	EntryID string `json:"entry_id"`
	Domain  string `json:"domain"`
	Title   string `json:"title"`
	// State is "loaded", "setup_error", "setup_retry", "not_loaded", ...
	State string `json:"state"`
	// Reason is why the entry is in its state, in Home Assistant's own words —
	// present on setup_retry and setup_error, and the message the config-entry
	// surface carries.
	Reason string `json:"reason"`
}

// repairIssue is one entry of the issue registry, as repairs/list_issues
// returns it. Severity is "critical", "error" or "warning".
type repairIssue struct {
	Domain         string            `json:"domain"`
	IssueID        string            `json:"issue_id"`
	Severity       string            `json:"severity"`
	IsFixable      bool              `json:"is_fixable"`
	Created        string            `json:"created"`
	TranslationKey string            `json:"translation_key"`
	Placeholders   map[string]string `json:"translation_placeholders"`
	LearnMoreURL   string            `json:"learn_more_url"`
	BreaksIn       string            `json:"breaks_in_ha_version"`
}

// entityState is one entity from get_states. Attributes stay raw: a handful are
// read here and the rest are the integration's own.
type entityState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged string         `json:"last_changed"`
}

// attr reads one attribute as text.
func (e *entityState) attr(name string) string {
	v, ok := e.Attributes[name]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// registryEntry maps an entity to the integration (platform) that owns it —
// the `integration` label a sensor condition groups by.
type registryEntry struct {
	EntityID string `json:"entity_id"`
	Platform string `json:"platform"`
}

// currentUser identifies the token this adapter authenticated with.
type currentUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// haMessage is the envelope every post-auth frame shares.
type haMessage struct {
	ID      int64           `json:"id,omitempty"`
	Type    string          `json:"type"`
	Success *bool           `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Event   json.RawMessage `json:"event,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// haSession is one authenticated connection. It owns a read loop; commands wait
// on it by id.
type haSession struct {
	conn *wsConn

	mu      sync.Mutex
	nextID  int64
	waiters map[int64]chan haMessage
	closed  bool

	// onEvent receives subscribed event data. Called from the read loop, so it
	// must not block.
	onEvent func(eventType string, data json.RawMessage)

	done chan struct{}
	err  error
}

// wsURLFor derives the WebSocket endpoint from the configured base URL.
func wsURLFor(endpoint string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("endpoint %q is not a URL: %w", endpoint, err)
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("endpoint %q must start with http:// or https://", endpoint)
	}
	if u.Host == "" {
		return "", fmt.Errorf("endpoint %q names no host", endpoint)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/websocket"
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

// haConnect dials, authenticates, and starts the read loop.
//
// The auth exchange is done inline rather than through the loop because the
// frames before auth_ok carry no id, so there is nothing for a waiter to key on.
func haConnect(ctx context.Context, endpoint, token string, onEvent func(string, json.RawMessage)) (*haSession, error) {
	wsURL, err := wsURLFor(endpoint)
	if err != nil {
		return nil, err
	}
	conn, err := wsDial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to Home Assistant at %s: %w", wsURL, err)
	}
	s := &haSession{
		conn:    conn,
		waiters: map[int64]chan haMessage{},
		onEvent: onEvent,
		done:    make(chan struct{}),
	}

	greet, err := s.readOne()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if greet.Type != "auth_required" {
		conn.Close()
		return nil, fmt.Errorf("Home Assistant greeted with %q, expected auth_required", greet.Type)
	}
	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": token}); err != nil {
		conn.Close()
		return nil, err
	}
	authed, err := s.readOne()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if authed.Type != "auth_ok" {
		conn.Close()
		msg := "authentication rejected"
		if authed.Error != nil {
			msg = authed.Error.Message
		}
		// A rejected token is a configuration fact, not a transient failure:
		// retrying it forever would look exactly like an unreachable host.
		return nil, fmt.Errorf("Home Assistant rejected the access token: %s", msg)
	}
	go s.readLoop()
	go s.pingLoop()
	return s, nil
}

func (s *haSession) readOne() (haMessage, error) {
	raw, err := s.conn.ReadMessage(time.Now().Add(haReadTimeout))
	if err != nil {
		return haMessage{}, err
	}
	var m haMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return haMessage{}, fmt.Errorf("decoding Home Assistant frame: %w", err)
	}
	return m, nil
}

func (s *haSession) readLoop() {
	defer close(s.done)
	for {
		m, err := s.readOne()
		if err != nil {
			s.fail(err)
			return
		}
		switch m.Type {
		case "event":
			if s.onEvent != nil {
				var env struct {
					EventType string          `json:"event_type"`
					Data      json.RawMessage `json:"data"`
				}
				if json.Unmarshal(m.Event, &env) == nil {
					s.onEvent(env.EventType, env.Data)
				}
			}
		default:
			s.deliver(m)
		}
	}
}

// pingLoop turns silence into a detectable failure. Without it a connection
// severed by a NAT timeout looks identical to a quiet house.
func (s *haSession) pingLoop() {
	t := time.NewTicker(haPingEvery)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), haCommandTimeout)
			_, err := s.Command(ctx, map[string]any{"type": "ping"})
			cancel()
			if err != nil {
				s.fail(err)
				return
			}
		}
	}
}

func (s *haSession) deliver(m haMessage) {
	s.mu.Lock()
	ch, ok := s.waiters[m.ID]
	delete(s.waiters, m.ID)
	s.mu.Unlock()
	if ok {
		ch <- m
	}
}

func (s *haSession) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.closed = true
	for id, ch := range s.waiters {
		close(ch)
		delete(s.waiters, id)
	}
	s.mu.Unlock()
	s.conn.Close()
}

// Command sends one request and waits for its result frame.
func (s *haSession) Command(ctx context.Context, msg map[string]any) (json.RawMessage, error) {
	s.mu.Lock()
	if s.closed {
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = errWSClosed
		}
		return nil, err
	}
	s.nextID++
	id := s.nextID
	ch := make(chan haMessage, 1)
	s.waiters[id] = ch
	s.mu.Unlock()

	out := map[string]any{"id": id}
	for k, v := range msg {
		out[k] = v
	}
	if err := s.conn.WriteJSON(out); err != nil {
		s.mu.Lock()
		delete(s.waiters, id)
		s.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.waiters, id)
		s.mu.Unlock()
		return nil, ctx.Err()
	case m, ok := <-ch:
		if !ok {
			if s.err != nil {
				return nil, s.err
			}
			return nil, errWSClosed
		}
		if m.Type == "pong" {
			return nil, nil
		}
		if m.Success != nil && !*m.Success {
			code, message := "unknown", "command failed"
			if m.Error != nil {
				code, message = m.Error.Code, m.Error.Message
			}
			return nil, fmt.Errorf("%v: %s (%s)", msg["type"], message, code)
		}
		return m.Result, nil
	}
}

// SubscribeEvents subscribes to one event type. Events arrive on the onEvent
// callback given at connect.
func (s *haSession) SubscribeEvents(ctx context.Context, eventType string) error {
	_, err := s.Command(ctx, map[string]any{"type": "subscribe_events", "event_type": eventType})
	return err
}

// SystemLog reads the current deduplicated log listing.
func (s *haSession) SystemLog(ctx context.Context) ([]logRecord, error) {
	raw, err := s.Command(ctx, map[string]any{"type": "system_log/list"})
	if err != nil {
		return nil, err
	}
	var out []logRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding system_log/list: %w", err)
	}
	return out, nil
}

// ConfigEntries reads the integration instances and their states.
//
// This is an ADMIN command. A non-admin token gets an error, which callers
// degrade to "cannot say" rather than treating as a failure — the verification
// ladder has a rung for exactly that.
func (s *haSession) ConfigEntries(ctx context.Context) ([]configEntry, error) {
	raw, err := s.Command(ctx, map[string]any{"type": "config_entries/get"})
	if err != nil {
		return nil, err
	}
	var out []configEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding config_entries/get: %w", err)
	}
	return out, nil
}

// Repairs lists the issue registry.
func (s *haSession) Repairs(ctx context.Context) ([]repairIssue, error) {
	raw, err := s.Command(ctx, map[string]any{"type": "repairs/list_issues"})
	if err != nil {
		return nil, err
	}
	var out struct {
		Issues []repairIssue `json:"issues"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding repairs/list_issues: %w", err)
	}
	return out.Issues, nil
}

// States reads every entity's state — the whole house, which is why it is on
// its own cadence.
func (s *haSession) States(ctx context.Context) ([]entityState, error) {
	raw, err := s.Command(ctx, map[string]any{"type": "get_states"})
	if err != nil {
		return nil, err
	}
	var out []entityState
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding get_states: %w", err)
	}
	return out, nil
}

// EntityRegistry reads which integration owns each entity.
func (s *haSession) EntityRegistry(ctx context.Context) ([]registryEntry, error) {
	raw, err := s.Command(ctx, map[string]any{"type": "config/entity_registry/list"})
	if err != nil {
		return nil, err
	}
	var out []registryEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding config/entity_registry/list: %w", err)
	}
	return out, nil
}

// CurrentUser identifies the token's user, feeding self-exclusion mechanism 2.
func (s *haSession) CurrentUser(ctx context.Context) (currentUser, error) {
	raw, err := s.Command(ctx, map[string]any{"type": "auth/current_user"})
	if err != nil {
		return currentUser{}, err
	}
	var out currentUser
	if err := json.Unmarshal(raw, &out); err != nil {
		return currentUser{}, fmt.Errorf("decoding auth/current_user: %w", err)
	}
	return out, nil
}

// Done closes when the session has failed or been closed.
func (s *haSession) Done() <-chan struct{} { return s.done }

// Err reports why the session ended.
func (s *haSession) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close ends the session.
func (s *haSession) Close() {
	s.fail(context.Canceled)
}
