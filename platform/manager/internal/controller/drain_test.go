package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// A node that is UNWELL is not a node that is being taken down. Kubernetes
// applies condition taints automatically, and reading them as a drain would
// release runtime pods during a transient NotReady — across many nodes at once
// during a partition, which is the worst possible moment to act on a stale view.
func TestNodeUnschedulable(t *testing.T) {
	taint := func(key string, effect corev1.TaintEffect) corev1.Taint {
		return corev1.Taint{Key: key, Effect: effect}
	}
	cases := []struct {
		name string
		node corev1.Node
		want bool
	}{
		{"a healthy node is not draining", corev1.Node{}, false},
		{
			"cordon sets the flag",
			corev1.Node{Spec: corev1.NodeSpec{Unschedulable: true}},
			true,
		},
		{
			"cordon's own taint counts even without the flag",
			corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				taint("node.kubernetes.io/unschedulable", corev1.TaintEffectNoSchedule)}}},
			true,
		},
		{
			"a maintenance taint counts",
			corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				taint("example.com/reboot-pending", corev1.TaintEffectNoSchedule)}}},
			true,
		},
		{
			"NotReady is a condition, not a drain",
			corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				taint("node.kubernetes.io/not-ready", corev1.TaintEffectNoSchedule)}}},
			false,
		},
		{
			"unreachable is a condition, not a drain",
			corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				taint("node.kubernetes.io/unreachable", corev1.TaintEffectNoExecute)}}},
			false,
		},
		{
			"disk pressure is a condition, not a drain",
			corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				taint("node.kubernetes.io/disk-pressure", corev1.TaintEffectNoSchedule)}}},
			false,
		},
		{
			"a PreferNoSchedule taint is a hint, not a drain",
			corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{
				taint("example.com/soft", corev1.TaintEffectPreferNoSchedule)}}},
			false,
		},
		{
			"an unwell node that is ALSO cordoned is draining",
			corev1.Node{Spec: corev1.NodeSpec{
				Unschedulable: true,
				Taints: []corev1.Taint{
					taint("node.kubernetes.io/not-ready", corev1.TaintEffectNoSchedule)}}},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeUnschedulable(&tc.node); got != tc.want {
				t.Fatalf("nodeUnschedulable = %v, want %v", got, tc.want)
			}
		})
	}
}
