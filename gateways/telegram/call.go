package main

import (
	"io"
	"log"
	"net/http"
	"strings"
)

// callMaxBody bounds an incoming call's body. sendDocument carries the whole
// attachment, so this is generous rather than the 4KiB a plain JSON call
// needs — the real Bot API applies its own, much larger, upload limit on top.
const callMaxBody = 20 << 20

// handleCall serves POST /telegram/call/{method}: the one path by which
// channel-telegram (or anything else downstream) reaches the real Bot API.
// This process is already the sole holder of the bot token for its stream —
// channel-telegram used to keep its own copy of it purely to send; this
// endpoint is what lets that copy go away. See gotchas.md, 2026-10-11.
//
// The body and its Content-Type are relayed VERBATIM in both directions, so a
// JSON method call and a multipart sendDocument upload take the same path
// with no gateway-side knowledge of which is which, and the caller's own
// tgResponse parsing — including `parameters.retry_after` — needs nothing
// new to read.
func (r *router) handleCall(w http.ResponseWriter, req *http.Request) {
	if !validCallToken(req, r.cfg.CallToken) {
		http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
		return
	}
	method := req.PathValue("method")
	if method == "" {
		http.Error(w, "method is required", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, callMaxBody))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	status, respBody, respCT, err := r.tg.Call(req.Context(), method, req.Header.Get("Content-Type"), body)
	if err != nil {
		log.Printf("telegram call %s: %v", method, err)
		http.Error(w, "upstream call failed", http.StatusBadGateway)
		return
	}
	if respCT != "" {
		w.Header().Set("Content-Type", respCT)
	}
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

// validCallToken checks the shared secret this leg carries, unlike /updates.
//
// /updates (gateway -> adapter) can only inject a FAKE INBOUND MESSAGE, which
// the receiving adapter still filters by chat id and approver before it does
// anything — network-level trust is enough there, consistent with this
// router holding no adapter token and sitting outside the adapter trust
// domain (telegram-ingest-router spec). This endpoint (adapter -> gateway) is
// a materially bigger grant: it can SEND, with the live bot credential, any
// Bot API method at all — including ones no inbound message could ever
// provoke, like deleting a chat's messages or rewriting its command menu. A
// pod that merely reaches this Service would otherwise act as the bot
// outright, so the token keeps that act restricted to a caller that was
// actually handed it.
func validCallToken(req *http.Request, want string) bool {
	const prefix = "Bearer "
	h := req.Header.Get("Authorization")
	return want != "" && strings.HasPrefix(h, prefix) && strings.TrimPrefix(h, prefix) == want
}
