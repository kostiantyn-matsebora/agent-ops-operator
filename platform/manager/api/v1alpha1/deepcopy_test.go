package v1alpha1

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func resourceQty(s string) resource.Quantity {
	return resource.MustParse(s)
}

// TestDeepCopyRoundTripsFuzzed is what actually exercises nearly every
// generated function: a hand-written or sample-derived literal only ever
// populates the fields someone thought of, so an optional pointer or a
// sub-struct nobody happened to set stays permanently uncovered. Fuzzing with
// NilChance(0) forces every pointer field down its non-nil DeepCopyInto
// branch, cascading into every nested Spec/Status type this API group has —
// the same technique apimachinery's own deepcopy-gen suite uses.
func newDeepCopyFuzzer(seed int64) *fuzz.Fuzzer {
	return fuzz.NewWithSeed(seed).NilChance(0).NumElements(1, 3).Funcs(
		// resource.Quantity caches a string form internally; a raw
		// reflection-fuzzed value can leave that cache inconsistent with the
		// numeric value, which DeepCopy preserves byte-for-byte and
		// reflect.DeepEqual would then flag as unequal for reasons that have
		// nothing to do with the generated code under test.
		func(q *resource.Quantity, c fuzz.Continue) {
			*q = resource.MustParse(fmt.Sprintf("%dm", c.Intn(1000)+1))
		},
		// metav1.Time wraps time.Time, whose monotonic reading component
		// survives a copy but not a comparison via reflect.DeepEqual in every
		// Go version; pinning to a wall-clock-only value sidesteps that.
		func(ts *metav1.Time, c fuzz.Continue) {
			*ts = metav1.NewTime(time.Unix(int64(c.Intn(1_000_000_000)), 0).UTC())
		},
		// RawExtension.Object is a runtime.Object INTERFACE; gofuzz can only
		// populate concrete types, so it is left nil (an ordinary state for
		// this type -- it carries Raw bytes OR a decoded Object, never both).
		func(re *runtime.RawExtension, c fuzz.Continue) {
			re.Raw = []byte(fmt.Sprintf(`{"n":%d}`, c.Intn(1000)))
			re.Object = nil
		},
	)
}

func TestDeepCopyRoundTripsFuzzed(t *testing.T) {
	f := newDeepCopyFuzzer(20260901)

	kinds := []any{
		&AgentProfile{}, &AgentProfileList{},
		&AgentRuntime{}, &AgentRuntimeList{},
		&Channel{}, &ChannelList{},
		&ChannelAdapter{}, &ChannelAdapterList{},
		&Conversation{}, &ConversationList{},
		&ConversationInput{}, &ConversationInputList{},
		&MCPConfig{}, &MCPConfigList{},
		&MCPToolset{}, &MCPToolsetList{},
		&Pipeline{}, &PipelineList{},
		&SignalAdapter{}, &SignalAdapterList{},
		&SignalSource{}, &SignalSourceList{},
	}
	for _, obj := range kinds {
		f.Fuzz(obj)
		roundTrip(t, obj)

		ro, ok := obj.(runtime.Object)
		if !ok {
			t.Fatalf("%T does not implement runtime.Object", obj)
		}
		if cp := ro.DeepCopyObject(); !reflect.DeepEqual(obj, cp) {
			t.Errorf("%T: DeepCopyObject produced a value different from the original", obj)
		}
	}
}

// TestDeepCopyRoundTripsSubTypesDirectly covers the standalone `DeepCopy()`
// convenience wrapper controller-gen writes for every Spec/Status/nested
// struct. A KIND's own DeepCopyInto reaches a value field's DeepCopyInto
// directly (never through that field's own DeepCopy() wrapper) and reaches a
// FLAT struct -- one with no pointer, slice or map field -- through a plain
// `*out = *in` with no generated call at all. Either way, nothing exercised by
// TestDeepCopyRoundTripsFuzzed above ever calls these wrappers, so they are
// called here, directly, on their own.
func TestDeepCopyRoundTripsSubTypesDirectly(t *testing.T) {
	f := newDeepCopyFuzzer(20260902)

	subTypes := []any{
		&AgentProfileSpec{}, &AgentProfileStatus{},
		&AgentRuntimeSpec{}, &AgentRuntimeStatus{},
		&ChannelSpec{}, &ChannelStatus{},
		&ChannelAdapterSpec{}, &ChannelAdapterStatus{},
		&ConversationSpec{}, &ConversationStatus{},
		&ConversationInputSpec{}, &ConversationInputStatus{},
		&MCPConfigSpec{}, &MCPConfigStatus{},
		&MCPToolsetSpec{},
		&PipelineSpec{}, &PipelineStatus{},
		&SignalAdapterSpec{}, &SignalAdapterStatus{},
		&SignalSourceSpec{}, &SignalSourceStatus{},
		&AdapterRef{}, &ContextCheckpoint{}, &ContextSync{}, &CooldownEntry{},
		&CredentialKeyDoc{}, &EgressMediation{}, &GroupingSpec{}, &InflightRun{},
		&InputItem{}, &InputOrigin{}, &NamedValue{}, &ObjectRef{}, &OriginReader{},
		&PersistenceBinding{}, &PipelinePersistence{}, &ReaderMark{},
		&RecordedInput{}, &RepoAuth{}, &RepositorySpec{}, &RunStatus{},
		&SignalProvenance{}, &ThreadBinding{}, &ToolingBinding{}, &ToolsetBinding{},
	}
	for _, obj := range subTypes {
		f.Fuzz(obj)
		roundTrip(t, obj)
	}
}

// Generated deepcopy code is exercised here rather than trusted unread: a
// nil-pointer or shared-slice bug in it corrupts every caller that mutates a
// copy expecting the original untouched, and nothing else in this module
// reaches most of it directly.

var yamlDocSeparator = regexp.MustCompile(`(?m)^---\s*$`)

type kindHeader struct {
	Kind string `json:"kind"`
}

// roundTrip unmarshals doc into obj, calls its DeepCopy() method by
// reflection (every generated top-level type has one, with a different
// concrete return type, so a shared helper needs reflection rather than a
// generic constraint), and checks the result is an equal but DISTINCT value.
func roundTrip(t *testing.T, obj any) {
	t.Helper()
	v := reflect.ValueOf(obj)
	m := v.MethodByName("DeepCopy")
	if !m.IsValid() {
		t.Fatalf("%T has no DeepCopy method", obj)
	}
	out := m.Call(nil)[0].Interface()
	if !reflect.DeepEqual(obj, out) {
		t.Errorf("%T: DeepCopy produced a value different from the original", obj)
	}
	if reflect.ValueOf(out).Pointer() == v.Pointer() {
		t.Errorf("%T: DeepCopy returned the SAME pointer, not a copy", obj)
	}
}

// TestDeepCopyRoundTripsEverySample decodes every CR the repository ships as a
// worked example and deep-copies it, which is what cascades into covering
// every nested Spec/Status DeepCopyInto too -- a sample populates far more
// optional fields than a minimal hand-written literal would.
func TestDeepCopyRoundTripsEverySample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "samples", "samples.yaml"))
	if err != nil {
		t.Fatalf("reading samples.yaml: %v", err)
	}
	docs := yamlDocSeparator.Split(string(raw), -1)
	tested := map[string]int{}
	for i, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var h kindHeader
		if err := yaml.Unmarshal([]byte(doc), &h); err != nil {
			t.Fatalf("doc %d: decoding kind: %v", i, err)
		}
		var obj any
		switch h.Kind {
		case "MCPConfig":
			obj = &MCPConfig{}
		case "MCPToolset":
			obj = &MCPToolset{}
		case "AgentProfile":
			obj = &AgentProfile{}
		case "Pipeline":
			obj = &Pipeline{}
		case "ChannelAdapter":
			obj = &ChannelAdapter{}
		case "Channel":
			obj = &Channel{}
		case "SignalAdapter":
			obj = &SignalAdapter{}
		case "SignalSource":
			obj = &SignalSource{}
		case "Conversation":
			obj = &Conversation{}
		default:
			t.Fatalf("doc %d: unhandled kind %q -- add a case so this sample is exercised too", i, h.Kind)
		}
		if err := yaml.Unmarshal([]byte(doc), obj); err != nil {
			t.Fatalf("doc %d (%s): unmarshalling: %v", i, h.Kind, err)
		}
		roundTrip(t, obj)
		tested[h.Kind]++
	}
	for _, k := range []string{
		"MCPConfig", "MCPToolset", "AgentProfile", "Pipeline", "ChannelAdapter",
		"Channel", "SignalAdapter", "SignalSource", "Conversation",
	} {
		if tested[k] == 0 {
			t.Errorf("no sample in samples.yaml exercised DeepCopy for %s", k)
		}
	}
}

// TestDeepCopyRoundTripsAgentRuntime covers the one kind samples.yaml carries
// no worked example of, populated so every optional/pointer/slice field's
// DeepCopyInto branch runs.
func TestDeepCopyRoundTripsAgentRuntime(t *testing.T) {
	dur := metav1.Duration{Duration: 5 * time.Minute}
	rt := &AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "agent-ops"},
		Spec: AgentRuntimeSpec{
			Image:              "ghcr.io/example/agentops-runtime-claude:1.0.0",
			Command:            []string{"/app"},
			Args:               []string{"--flag"},
			Env:                []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
			ServiceAccountName: "agentops-runtime",
			NodeSelector:       map[string]string{"kubernetes.io/hostname": "node-1"},
			IdleTTLMinutes:     10,
			ContextStorage:     ContextOnVolume,
			ContextSync: &ContextSync{
				Paths:    []string{"/data/context"},
				Exclude:  []string{"*.tmp"},
				Interval: &dur,
				Retain:   3,
			},
			EgressMediation: &EgressMediation{
				Port:         8080,
				ExcludePorts: []int32{53, 443},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceCPU: resourceQty("100m")},
				},
			},
			Resources: &corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resourceQty("256Mi")},
				Requests: corev1.ResourceList{corev1.ResourceMemory: resourceQty("128Mi")},
			},
		},
		Status: AgentRuntimeStatus{
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
		},
	}
	roundTrip(t, rt)

	list := &AgentRuntimeList{Items: []AgentRuntime{*rt}}
	roundTrip(t, list)
}

// TestDeepCopyRoundTripsConversationInput covers the other kind samples.yaml
// carries no worked example of.
func TestDeepCopyRoundTripsConversationInput(t *testing.T) {
	now := metav1.Now()
	ci := &ConversationInput{
		ObjectMeta: metav1.ObjectMeta{Name: "conv-1-abc", Namespace: "agent-ops"},
		Spec: ConversationInputSpec{
			ConversationRef: ObjectRef{Name: "conv-1"},
			Type:            InputTask,
			Payload:         "do the thing",
			Labels:          map[string]string{"agentops.dev/channel": "telegram"},
		},
		Status: ConversationInputStatus{
			Consumed:   true,
			ConsumedAt: &now,
		},
	}
	roundTrip(t, ci)

	list := &ConversationInputList{Items: []ConversationInput{*ci}}
	roundTrip(t, list)
}
