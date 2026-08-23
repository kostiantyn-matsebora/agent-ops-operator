package v1alpha1

import "testing"

// The rename must not do the harm the field exists to prevent. A spec field
// that simply moved would strand every installed runtime at the moment of
// upgrade — its pods would come back with an empty volume and every
// conversation in the install would answer without its context while every
// signal reported success.

func TestContextVolumeDualRead(t *testing.T) {
	ref := func(n string) *ObjectRef { return &ObjectRef{Name: n} }

	cases := []struct {
		name string
		spec AgentRuntimeSpec
		want string // resolved claim name, "" for none
		nil_ bool
	}{
		{
			name: "the current field wins where both are present",
			spec: AgentRuntimeSpec{
				Context: &ContextVolume{PVCRef: ref("agentops-context")},
				Home:    &HomeVolume{PVCRef: ref("agentops-home")},
			},
			want: "agentops-context",
		},
		{
			name: "a runtime from before the rename is honoured",
			spec: AgentRuntimeSpec{Home: &HomeVolume{PVCRef: ref("agentops-home")}},
			want: "agentops-home",
		},
		{
			name: "the current field alone",
			spec: AgentRuntimeSpec{Context: &ContextVolume{PVCRef: ref("agentops-context")}},
			want: "agentops-context",
		},
		{
			name: "neither declared",
			spec: AgentRuntimeSpec{},
			nil_: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.ContextVolume()
			if tc.nil_ {
				if got != nil {
					t.Fatalf("ContextVolume() = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.PVCRef == nil {
				t.Fatalf("ContextVolume() = %+v, want claim %q", got, tc.want)
			}
			if got.PVCRef.Name != tc.want {
				t.Fatalf("claim = %q, want %q", got.PVCRef.Name, tc.want)
			}
		})
	}
}

// emptyDir is the other half of the shape, and a runtime declaring it under
// the retired name is declaring the same thing.
func TestContextVolumeCarriesEmptyDirThroughTheRename(t *testing.T) {
	spec := AgentRuntimeSpec{Home: &HomeVolume{EmptyDir: true}}
	got := spec.ContextVolume()
	if got == nil || !got.EmptyDir {
		t.Fatalf("ContextVolume() = %+v, want emptyDir", got)
	}
}
