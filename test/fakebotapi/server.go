// Package main is a FAKE Telegram Bot API server for the end-to-end pack and
// the conformance suite. TEST ONLY — it is never a component
// (.github/components.sh excludes test/), and it identifies itself as a fake in
// every response header so its presence anywhere real is obvious.
//
// It is faithful where faithfulness matters: gateway-telegram forwards an
// Update VERBATIM, so an Update fed to this server through the control API
// reaches signal-telegram and channel-telegram byte-identical to what the real
// API would have produced. And it refuses a second concurrent getUpdates with
// the same 409 Telegram sends, which is what lets the pack assert the
// single-consumer invariant rather than assume it.
//
// Bot API surface (POST /bot<token>/<method>, JSON or multipart):
//
//	getUpdates        long-polls the scripted queue; offset semantics as Telegram's
//	sendMessage       records the call, returns a Message with a fresh message_id
//	sendDocument      multipart; records fields plus the document's filename
//	createForumTopic  returns a ForumTopic with a fresh message_thread_id
//	closeForumTopic   returns true
//	<anything else>   recorded, answered {"ok":true,"result":true}
//
// Control surface (never under /bot):
//
//	POST   /control/updates      body: one Update object or an array; queued in order
//	GET    /control/calls        every recorded call, oldest first; ?method= filters
//	DELETE /control/calls        forget recorded calls
//	GET    /control/consumers    {"active":n,"maxConcurrent":m,"conflicts":c} for getUpdates
//	POST   /control/fail         inject a 429/5xx rejection ahead of the next real
//	                             answer for one method — body {"method","mode"
//	                             ("429"|"5xx"),"retryAfter","forever"}; forever=false
//	                             (default) queues ONE rejection, forever=true makes
//	                             every call to method fail until cleared
//	DELETE /control/fail         ?method= clears both the queue and the forever rule
//	GET    /healthz
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Call is one recorded Bot API call.
type Call struct {
	Token  string         `json:"token"`
	Method string         `json:"method"`
	Body   map[string]any `json:"body,omitempty"`
	// Document is the uploaded file's name on sendDocument, and its size.
	Document *Document `json:"document,omitempty"`
	At       time.Time `json:"at"`
}

// Document describes an uploaded file without keeping its bytes.
type Document struct {
	Filename string `json:"filename"`
	Size     int    `json:"size"`
}

// Server is the fake. Safe for concurrent use.
type Server struct {
	mu       sync.Mutex
	cond     *sync.Cond
	queue    []json.RawMessage // scripted updates, in order
	nextID   int64             // next update_id assigned to an unnumbered update
	calls    []Call
	msgID    int64
	threadID int64

	// consumers tracks getUpdates long-polls in flight per token. Telegram
	// serves ONE stream per token; the second poller gets 409, and so does
	// this fake, because the single-consumer rule is an invariant the pack
	// must be able to see broken.
	polling       map[string]int
	maxConcurrent int
	conflicts     int

	// failQueue holds one-shot injected failures per method, consumed FIFO —
	// a conformance assertion over the per-operation retry budget (A1/A2)
	// needs a REAL 429/5xx off the wire, not a client-side mock, so the
	// backpressure a caller absorbs is the same shape the real Bot API sends.
	// failForever is a sticky rule served once the queue is empty, for
	// "backpressure that never clears".
	failQueue   map[string][]failRule
	failForever map[string]failRule
}

// failRule is one injected Bot API rejection.
type failRule struct {
	Mode       string // "429" or "5xx"
	RetryAfter int    // seconds; 429 only
}

// New builds an empty fake.
func New() *Server {
	s := &Server{nextID: 1, polling: map[string]int{}, failQueue: map[string][]failRule{}, failForever: map[string]failRule{}}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Handler is the whole HTTP surface.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /control/updates", s.handleFeed)
	mux.HandleFunc("GET /control/calls", s.handleCalls)
	mux.HandleFunc("DELETE /control/calls", s.handleReset)
	mux.HandleFunc("GET /control/consumers", s.handleConsumers)
	mux.HandleFunc("POST /control/fail", s.handleFail)
	mux.HandleFunc("DELETE /control/fail", s.handleClearFail)
	// A Bot API path is /bot<token>/<method>; ServeMux wildcards must be whole
	// segments, so the split is done by hand.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, "/bot")
		token, method, found := strings.Cut(rest, "/")
		if !ok || !found || token == "" || method == "" {
			http.NotFound(w, r)
			return
		}
		s.handleBot(w, r, token, method)
	})
	return fakeHeader(mux)
}

// fakeHeader stamps every response so a real component pointed here by
// mistake can be told apart from Telegram in one line of a capture.
func fakeHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Agentops-Fake-Bot-Api", "true")
		next.ServeHTTP(w, r)
	})
}

// Feed queues updates, assigning update_id to any that lack one so the
// offset protocol works for hand-written fixtures too.
func (s *Server) Feed(raws ...json.RawMessage) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	for _, raw := range raws {
		var probe struct {
			UpdateID *int64 `json:"update_id"`
		}
		_ = json.Unmarshal(raw, &probe)
		id := s.nextID
		if probe.UpdateID != nil {
			id = *probe.UpdateID
		} else {
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			if m == nil {
				m = map[string]any{}
			}
			m["update_id"] = id
			raw, _ = json.Marshal(m)
		}
		if id >= s.nextID {
			s.nextID = id + 1
		}
		s.queue = append(s.queue, raw)
		ids = append(ids, id)
	}
	s.cond.Broadcast()
	return ids
}

// Calls returns the recorded calls, optionally filtered by method.
func (s *Server) Calls(method string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Call
	for _, c := range s.calls {
		if method == "" || c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	trimmed := strings.TrimSpace(string(body))
	var raws []json.RawMessage
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &raws); err != nil {
			http.Error(w, "updates must be a JSON array or object: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if !json.Valid(body) {
			http.Error(w, "update must be JSON", http.StatusBadRequest)
			return
		}
		raws = []json.RawMessage{json.RawMessage(body)}
	}
	ids := s.Feed(raws...)
	writeJSON(w, http.StatusOK, map[string]any{"queued": ids})
}

func (s *Server) handleCalls(w http.ResponseWriter, r *http.Request) {
	calls := s.Calls(r.URL.Query().Get("method"))
	if calls == nil {
		calls = []Call{}
	}
	writeJSON(w, http.StatusOK, calls)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.calls = nil
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleConsumers(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	active := 0
	for _, n := range s.polling {
		active += n
	}
	out := map[string]int{"active": active, "maxConcurrent": s.maxConcurrent, "conflicts": s.conflicts}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Method     string `json:"method"`
		Mode       string `json:"mode"`
		RetryAfter int    `json:"retryAfter"`
		Forever    bool   `json:"forever"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Method == "" {
		http.Error(w, "method is required", http.StatusBadRequest)
		return
	}
	rule := failRule{Mode: in.Mode, RetryAfter: in.RetryAfter}
	s.mu.Lock()
	if in.Forever {
		s.failForever[in.Method] = rule
	} else {
		s.failQueue[in.Method] = append(s.failQueue[in.Method], rule)
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearFail(w http.ResponseWriter, r *http.Request) {
	method := r.URL.Query().Get("method")
	s.mu.Lock()
	delete(s.failQueue, method)
	delete(s.failForever, method)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// consumeFail returns the next injected failure for method, if any, draining
// a one-shot queue entry before falling back to a sticky "forever" rule.
func (s *Server) consumeFail(method string) (failRule, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q := s.failQueue[method]; len(q) > 0 {
		s.failQueue[method] = q[1:]
		return q[0], true
	}
	if r, ok := s.failForever[method]; ok {
		return r, true
	}
	return failRule{}, false
}

// writeFail answers call AS Telegram would reject it — a real 429 with a
// stated retry_after, or a bare 5xx — so a caller's retry loop sees the exact
// shape it has to absorb from the real API, never a test-side shortcut.
func writeFail(w http.ResponseWriter, rule failRule) {
	switch rule.Mode {
	case "429":
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok": false, "error_code": http.StatusTooManyRequests,
			"description": fmt.Sprintf("Too Many Requests: retry after %d", rule.RetryAfter),
			"parameters":  map[string]any{"retry_after": rule.RetryAfter},
		})
	default: // "5xx"
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok": false, "error_code": http.StatusInternalServerError,
			"description": "Internal Server Error",
		})
	}
}

func (s *Server) handleBot(w http.ResponseWriter, r *http.Request, token, method string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	call := Call{Token: token, Method: method, At: time.Now().UTC(), Body: map[string]any{}}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch {
	case strings.HasPrefix(ct, "multipart/"):
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			botError(w, 400, "Bad Request: "+err.Error())
			return
		}
		for k, v := range r.MultipartForm.Value {
			if len(v) > 0 {
				call.Body[k] = v[0]
			}
		}
		if files := r.MultipartForm.File["document"]; len(files) > 0 {
			call.Document = &Document{Filename: files[0].Filename, Size: int(files[0].Size)}
		}
	default:
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &call.Body); err != nil {
				botError(w, 400, "Bad Request: body is not JSON")
				return
			}
		}
		for k, v := range r.URL.Query() {
			if _, ok := call.Body[k]; !ok && len(v) > 0 {
				call.Body[k] = v[0]
			}
		}
	}

	// Injected backpressure pre-empts every outbound method — never
	// getUpdates, which has no retry budget to pin and whose own long-poll
	// semantics are unrelated.
	if method != "getUpdates" {
		if rule, failing := s.consumeFail(method); failing {
			s.record(call)
			writeFail(w, rule)
			return
		}
	}

	switch method {
	case "getUpdates":
		s.getUpdates(w, r, token, call)
	case "sendMessage":
		s.record(call)
		id := s.next(&s.msgID)
		res := map[string]any{"message_id": id, "date": time.Now().Unix(),
			"chat": map[string]any{"id": call.Body["chat_id"], "type": "supergroup"},
			"text": call.Body["text"]}
		if t, ok := call.Body["message_thread_id"]; ok {
			res["message_thread_id"] = t
		}
		botOK(w, res)
	case "sendDocument":
		s.record(call)
		id := s.next(&s.msgID)
		res := map[string]any{"message_id": id, "date": time.Now().Unix(),
			"chat": map[string]any{"id": call.Body["chat_id"], "type": "supergroup"}}
		if call.Document != nil {
			res["document"] = map[string]any{"file_name": call.Document.Filename, "file_id": "fake-file-" + strconv.FormatInt(id, 10)}
		}
		botOK(w, res)
	case "createForumTopic":
		s.record(call)
		id := s.next(&s.threadID)
		botOK(w, map[string]any{"message_thread_id": id, "name": call.Body["name"], "icon_color": 7322096})
	case "closeForumTopic", "reopenForumTopic", "deleteForumTopic", "setMyCommands",
		"deleteMyCommands", "answerCallbackQuery", "editMessageText", "deleteMessage":
		s.record(call)
		botOK(w, true)
	default:
		s.record(call)
		botOK(w, true)
	}
}

func (s *Server) record(c Call) {
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
}

func (s *Server) next(counter *int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	*counter++
	return *counter
}

// getUpdates serves the queue with Telegram's offset semantics: updates with
// update_id >= offset, and a call with offset N confirms everything below N.
// It long-polls for `timeout` seconds when nothing is queued. A SECOND
// concurrent poller on the same token is answered 409, as Telegram does.
func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request, token string, call Call) {
	offset := asInt(call.Body["offset"])
	timeout := asInt(call.Body["timeout"])
	if timeout > 30 {
		timeout = 30
	}
	s.mu.Lock()
	if s.polling[token] > 0 {
		s.conflicts++
		s.mu.Unlock()
		s.record(call)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 409,
			"description": "Conflict: terminated by other getUpdates request; make sure that only one bot instance is running"})
		return
	}
	s.polling[token]++
	if s.polling[token] > s.maxConcurrent {
		s.maxConcurrent = s.polling[token]
	}
	// Confirm: drop everything below the offset.
	if offset > 0 {
		kept := s.queue[:0]
		for _, raw := range s.queue {
			if updateID(raw) >= offset {
				kept = append(kept, raw)
			}
		}
		s.queue = kept
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	// Wake the wait on a deadline or a client disconnect; cond has no timeout.
	// The waker is released as soon as the poll is answered.
	served := make(chan struct{})
	defer close(served)
	go func() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
		case <-served:
			return
		}
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
	}()
	for len(s.queue) == 0 && time.Now().Before(deadline) && r.Context().Err() == nil {
		s.cond.Wait()
	}
	var out []json.RawMessage
	for _, raw := range s.queue {
		if offset <= 0 || updateID(raw) >= offset {
			out = append(out, raw)
		}
	}
	s.polling[token]--
	s.mu.Unlock()
	s.record(call)
	if out == nil {
		out = []json.RawMessage{}
	}
	botOK(w, out)
}

func updateID(raw json.RawMessage) int64 {
	var probe struct {
		UpdateID int64 `json:"update_id"`
	}
	_ = json.Unmarshal(raw, &probe)
	return probe.UpdateID
}

func asInt(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

func botOK(w http.ResponseWriter, result any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

func botError(w http.ResponseWriter, code int, desc string) {
	writeJSON(w, code, map[string]any{"ok": false, "error_code": code, "description": desc})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
