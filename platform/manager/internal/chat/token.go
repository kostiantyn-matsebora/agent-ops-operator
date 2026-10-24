package chat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// DeriveAdapterToken derives a ChannelAdapter's contract bearer token from the
// master key (ADAPTER_TOKEN env): HMAC-SHA256(masterKey, "adapter:"+name),
// base64url. Stateless by design — the reconciler injects it into the adapter
// pod and the manager validates any presented token by re-derivation against
// the ChannelAdapter list, so nothing is minted, stored, or read back from
// Secrets, and validation survives manager restarts.
func DeriveAdapterToken(masterKey, adapterName string) string {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("adapter:" + adapterName))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// DeriveSignalAdapterToken is the SignalAdapter sibling of DeriveAdapterToken
// with a distinct derivation context, so a ChannelAdapter and a SignalAdapter
// sharing a name never share a token (each surface validates only against its
// own CRD list).
func DeriveSignalAdapterToken(masterKey, adapterName string) string {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("signal-adapter:" + adapterName))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// DeriveCoordinatorToken derives the PER-CONVERSATION token a coordinated
// conversation's runtime pod holds (design D-F, `aops-mcp-server`): HMAC of
// the master key and `coordinator:<coordinatorName>:<conversationName>`.
//
// Per CONVERSATION rather than per Coordinator, because one Coordinator may
// hold several open conversations at once, nested or not, and a token naming
// only the Coordinator could not scope reach to one of them. The manager
// validates a presented token by re-deriving it against every conversation
// carrying `spec.coordinatorRef` — the same re-derivation pattern
// DeriveAdapterToken already uses against the ChannelAdapter list — so
// nothing is minted or stored anywhere.
func DeriveCoordinatorToken(masterKey, coordinatorName, conversationName string) string {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("coordinator:" + coordinatorName + ":" + conversationName))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// DeriveChannelReaderToken derives the SECOND reach class `aops-mcp-server`
// defines: a token scoped to one Channel, reaching only the
// `{name,title,brief,phase,pipeline}` projection of conversations bound to it
// and refused for every verb. Distinct context from DeriveCoordinatorToken so
// a Channel and a Coordinator sharing a name never share a token.
func DeriveChannelReaderToken(masterKey, channelName string) string {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("channel-reader:" + channelName))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
