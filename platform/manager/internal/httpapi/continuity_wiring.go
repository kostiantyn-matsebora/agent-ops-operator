package httpapi

import "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/storagebreaker"

// breaker lazily builds the storage breaker, so a zero-valued Server (tests)
// works without wiring one. When the manager wires a shared instance, this
// returns that instance and both edges — reports and provisioning — feed it.
func (s *Server) breaker() *storagebreaker.Breaker {
	s.breakerOnce.Do(func() {
		if s.StorageBreaker == nil {
			s.StorageBreaker = storagebreaker.New()
		}
	})
	return s.StorageBreaker
}
