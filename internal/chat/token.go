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
