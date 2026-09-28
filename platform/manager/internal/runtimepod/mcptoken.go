package runtimepod

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// DeriveCoordinatorToken is chat.DeriveCoordinatorToken, DUPLICATED rather
// than imported: chat already imports runtimepod (it builds a member's
// persistence snapshot), so the reverse import would cycle — the same shape
// labelSignatureHash is duplicated across chat and controller for. Exported
// so internal/controller, the one package that can import both, can pin the
// two against each other. Both sides are pinned by
// TestDeriveCoordinatorTokenMatchesChatPackage.
func DeriveCoordinatorToken(masterKey, coordinatorName, conversationName string) string {
	mac := hmac.New(sha256.New, []byte(masterKey))
	mac.Write([]byte("coordinator:" + coordinatorName + ":" + conversationName))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
