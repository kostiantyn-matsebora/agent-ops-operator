package controller

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// A runtime pod that never STARTS used to be immortal.
//
// Reaping handled Succeeded and Failed; Pending was counted as active —
// correctly, since a stuck pod must not invent capacity — but nothing bounded
// how long it could sit there. On 2026-08-20 a corrupt filesystem on the shared
// home volume left five pods in ContainerCreating for fifteen hours. They held
// every slot, starved six conversations behind them, and the install reported
// NOTHING: no condition, no event, no log line. The only condition present said
// DeliveryPending=False / AllDelivered, which reads as healthy.
//
// This file is the evidence half of the fix. The reaping half is in the
// reconciler; what makes either useful is that the REASON reaches an operator,
// so a deadline that recorded only "deadline exceeded" would reproduce the
// original failure with a timer attached.

// DefaultRuntimeStartDeadline bounds how long a runtime pod may sit without
// reaching Running.
//
// Generous on purpose: it must clear a cold image pull of a multi-gigabyte
// runtime image on a slow link, because reaping a pod that was merely still
// pulling would turn a slow start into a restart loop that never finishes one.
// The failures it is aimed at do not resolve in ten minutes or in ten hours.
const DefaultRuntimeStartDeadline = 10 * time.Minute

// runtimeStartBackoffBase and runtimeStartBackoffMax bound the delay between
// attempts after a pod fails to start. Doubling from the base, capped — an
// environmental failure must not be retried at reconcile speed, and must not
// back off so far that recovery goes unnoticed for an hour.
const (
	runtimeStartBackoffBase = 30 * time.Second
	runtimeStartBackoffMax  = 10 * time.Minute
)

// Reasons for the RuntimeStarted condition. They are also the classification:
// only ReasonVolumeUnavailable is attributable to context storage, and only
// that one may be offered to the storage breaker.
const (
	// ReasonVolumeUnavailable: the pod's volumes never became usable. This is
	// the storage-attributable case.
	ReasonVolumeUnavailable = "VolumeUnavailable"
	// ReasonUnschedulable: no node would take the pod.
	ReasonUnschedulable = "Unschedulable"
	// ReasonImageUnavailable: the image could not be pulled.
	ReasonImageUnavailable = "ImageUnavailable"
	// ReasonNotStarted: past the deadline for a reason the pod does not
	// classify. Named rather than guessed — a wrong attribution is worse than
	// an honest "did not start", because it sends the reader somewhere else.
	ReasonNotStarted = "NotStarted"
	// ReasonStarted: the pod reached Running.
	ReasonStarted = "Started"
)

// startFailure is what a stuck pod says about itself.
type startFailure struct {
	// Reason is the condition reason, one of the Reason* constants.
	Reason string
	// Message is the pod's OWN evidence, quoted rather than paraphrased. The
	// manager does not diagnose storage; it reports what the kubelet said.
	Message string
	// Storage reports whether this failure is attributable to context storage,
	// and therefore whether it may count toward treating storage as an outage.
	// A pod that cannot be scheduled or cannot pull an image is a different
	// fault, and letting it open a STORAGE breaker would hold every
	// conversation for a reason that has nothing to do with storage.
	Storage bool
}

// podStarted reports whether a runtime pod has got far enough that the start
// deadline no longer applies. Succeeded and Failed count: both are handled by
// the existing exit reaping, and a pod that ran and exited did start.
func podStarted(pod *corev1.Pod) bool {
	switch pod.Status.Phase {
	case corev1.PodRunning, corev1.PodSucceeded, corev1.PodFailed:
		return true
	}
	return false
}

// runtimeStartOverdue reports whether a pod that has not started is past its
// deadline.
//
// Measured from the pod's CREATION, not from status.startTime: startTime is set
// when the kubelet accepts the pod, and a pod that no kubelet ever accepted —
// the unschedulable case — would otherwise never become overdue at all.
func runtimeStartOverdue(pod *corev1.Pod, deadline time.Duration, now time.Time) bool {
	if podStarted(pod) {
		return false
	}
	return now.Sub(pod.CreationTimestamp.Time) > deadline
}

// classifyStuckPod reads a pod's own status for why it has not started.
//
// Deliberately NOT event-driven. The manager holds `create` and `patch` on
// events and no read verbs at all, and granting it namespace-wide event reads
// to improve one message would be a real privilege expansion for a cosmetic
// gain. Everything needed is on the pod, which the reconciler already watches:
//
//   - PodScheduled=False means no node took it.
//   - PodReadyToStartContainers=False means the sandbox and its VOLUMES are not
//     ready. This is the discriminator the outage turned on — it stays False
//     exactly while a volume will not attach, and it flips True before image
//     pulling begins, so a slow pull can never be mistaken for a bad volume.
//   - A waiting container reason names an image problem when there is one.
func classifyStuckPod(pod *corev1.Pod) startFailure {
	if c := podCondition(pod, corev1.PodScheduled); c != nil && c.Status == corev1.ConditionFalse {
		return startFailure{
			Reason:  ReasonUnschedulable,
			Message: condEvidence("pod is not scheduled", c),
			Storage: false,
		}
	}
	if reason, msg, ok := imageProblem(pod); ok {
		return startFailure{
			Reason:  ReasonImageUnavailable,
			Message: strings.TrimSpace(fmt.Sprintf("container image unavailable (%s): %s", reason, msg)),
			Storage: false,
		}
	}
	if c := podCondition(pod, corev1.PodReadyToStartContainers); c != nil && c.Status == corev1.ConditionFalse {
		return startFailure{
			Reason: ReasonVolumeUnavailable,
			Message: condEvidence(
				"pod is not ready to start containers, which is what an unattached or unmountable volume looks like",
				c) + waitingSuffix(pod) + mediationSuffix(pod),
			Storage: true,
		}
	}
	return startFailure{
		Reason:  ReasonNotStarted,
		Message: "runtime pod did not reach Running before its start deadline" +
			waitingSuffix(pod) + mediationSuffix(pod),
		Storage: false,
	}
}

// podCondition returns a pod condition by type, or nil.
func podCondition(pod *corev1.Pod, t corev1.PodConditionType) *corev1.PodCondition {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == t {
			return &pod.Status.Conditions[i]
		}
	}
	return nil
}

// condEvidence renders a condition as prose plus whatever the kubelet actually
// wrote, so the reader gets the raw string and not only our reading of it.
func condEvidence(lead string, c *corev1.PodCondition) string {
	var b strings.Builder
	b.WriteString(lead)
	if c.Reason != "" {
		fmt.Fprintf(&b, " [%s]", c.Reason)
	}
	if m := strings.TrimSpace(c.Message); m != "" {
		fmt.Fprintf(&b, ": %s", m)
	}
	return b.String()
}

// imagePullReasons are the waiting reasons that mean the image, not the volume.
var imagePullReasons = map[string]bool{
	"ImagePullBackOff":         true,
	"ErrImagePull":             true,
	"InvalidImageName":         true,
	"ImageInspectError":   true,
	"RegistryUnavailable": true,
	// CreateContainerConfigError is deliberately absent: it means a referenced
	// ConfigMap or Secret is missing, not that the image is. Filing it here
	// would send the reader to a registry over a wiring problem, and the honest
	// NotStarted fallback names it without guessing.
}

// mediationSuffix names the mediation containers when they are the ones stuck.
//
// The classifier already scans init container statuses, so a proxy that cannot
// pull or will not start already fails the pod with the kubelet's own reason.
// What it could not say is WHICH container, and that matters here more than
// usual: "init container waiting" sends an operator to the runtime image, while
// the actual answer is that the enforcing proxy is the thing that did not come
// up — and until it does, the pod fails closed by design rather than by fault.
func mediationSuffix(pod *corev1.Pod) string {
	var stuck []string
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name != "egress-proxy" && cs.Name != "egress-init" {
			continue
		}
		if cs.State.Waiting != nil || (cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0) {
			stuck = append(stuck, cs.Name)
		}
	}
	if len(stuck) == 0 {
		return ""
	}
	sort.Strings(stuck)
	return " (egress mediation: " + strings.Join(stuck, ", ") +
		" has not started, so the agent reaches nothing until it does)"
}

// imageProblem reports an image-related waiting reason on any container.
func imageProblem(pod *corev1.Pod) (reason, message string, ok bool) {
	all := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...),
		pod.Status.ContainerStatuses...)
	for _, cs := range all {
		w := cs.State.Waiting
		if w != nil && imagePullReasons[w.Reason] {
			return w.Reason, strings.TrimSpace(w.Message), true
		}
	}
	return "", "", false
}

// waitingSuffix appends the containers' own waiting reasons, deduplicated and
// ordered, so the message carries the kubelet's vocabulary rather than ours.
func waitingSuffix(pod *corev1.Pod) string {
	seen := map[string]bool{}
	all := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...),
		pod.Status.ContainerStatuses...)
	for _, cs := range all {
		if w := cs.State.Waiting; w != nil && w.Reason != "" {
			seen[w.Reason] = true
		}
	}
	if len(seen) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(seen))
	for r := range seen {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons) // stable: a message that reshuffles reads as a change
	return " (containers waiting: " + strings.Join(reasons, ", ") + ")"
}

// runtimeStartBackoff is how long to wait after `failures` consecutive failed
// starts. Derived from the count, never stored, so there is one source of truth
// for when the next attempt is due.
func runtimeStartBackoff(failures int32) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := runtimeStartBackoffBase
	for i := int32(1); i < failures; i++ {
		d *= 2
		if d >= runtimeStartBackoffMax {
			return runtimeStartBackoffMax
		}
	}
	return d
}
