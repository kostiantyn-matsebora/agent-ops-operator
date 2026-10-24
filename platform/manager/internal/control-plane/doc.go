// Package controlplane is the manager's /control-plane/* HTTP surface: one
// named, additive door for operational configuration, authenticated by its
// own dedicated bearer token (CONTROL_PLANE_TOKEN) — isolated from
// ADAPTER_TOKEN and the coordinator token, never derived from or compared
// against either (see Auth's doc comment).
//
// Routes, every one guarded by Auth:
//
//	PUT /control-plane/probes/{name}   create-or-update by name
//	GET /control-plane/probes/{name}   read by name, 404 on an unknown name
//
// ControlPlaneProbe (platform/manager/api/v1alpha1) is explicitly disposable
// scaffolding, proving the token-auth -> handler -> object-CRUD round trip
// this surface exists for. It is deleted or renamed, never grown, once a
// real first resident is proposed.
//
// RegisterRoutes is where every kind this surface hosts gets wired, one
// route pair per kind. A path this package has no handler for falls through
// to a 404 that creates, reads or modifies nothing — a request for an
// unhandled kind must find nothing, exactly as if the route did not exist.
//
// RBAC pattern every resident of this surface follows (design.md): the
// manager's own Role holds full CRUD on it, every other component's Role
// stays read-only — get/list/watch — or holds nothing, never a write verb.
// See chart/templates/rbac.yaml and docs/concepts.md.
package controlplane
