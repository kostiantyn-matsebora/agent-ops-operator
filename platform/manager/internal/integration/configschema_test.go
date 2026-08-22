package integration

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
)

const chatSchema = `{"type":"object","properties":{"chatId":{"type":"string"},` +
	`"feedThreadId":{"type":"integer"}},"required":["chatId"]}`

func setChannelAdapterSchema(t *testing.T, name, schema string) {
	t.Helper()
	ctx := context.Background()
	var a agentopsv1alpha1.ChannelAdapter
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &a); err != nil {
		t.Fatal(err)
	}
	if schema == "" {
		a.Spec.ConfigSchema = nil
	} else {
		a.Spec.ConfigSchema = &runtime.RawExtension{Raw: []byte(schema)}
	}
	if err := k8sClient.Update(ctx, &a); err != nil {
		t.Fatal(err)
	}
}

// channelCondition returns a Channel's condition, or nil when absent.
func channelCondition(t *testing.T, name, condType string) *conditionView {
	t.Helper()
	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &ch); err != nil {
		t.Fatal(err)
	}
	c := apimeta.FindStatusCondition(ch.Status.Conditions, condType)
	if c == nil {
		return nil
	}
	return &conditionView{Status: string(c.Status), Reason: c.Reason, Message: c.Message}
}

type conditionView struct{ Status, Reason, Message string }

// The declaration is optional and compile-checked where it is authored; a
// broken schema must never take the adapter workload down with it.
func TestAdapterSchemaCompileCheck(t *testing.T) {
	ctx := context.Background()
	mkAdapter(t, "sch-adapter")
	reconcileAdapter(t, "sch-adapter")

	var a agentopsv1alpha1.ChannelAdapter
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "sch-adapter"}, &a)
	if apimeta.FindStatusCondition(a.Status.Conditions, controller.ConditionSchemaValid) != nil {
		t.Fatal("no declaration must mean no SchemaValid condition")
	}

	setChannelAdapterSchema(t, "sch-adapter", chatSchema)
	reconcileAdapter(t, "sch-adapter")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "sch-adapter"}, &a)
	if !apimeta.IsStatusConditionTrue(a.Status.Conditions, controller.ConditionSchemaValid) {
		t.Fatalf("compilable schema should report SchemaValid=True: %+v", a.Status.Conditions)
	}

	setChannelAdapterSchema(t, "sch-adapter", `{"type":"objekt"}`)
	reconcileAdapter(t, "sch-adapter")
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "sch-adapter"}, &a)
	c := apimeta.FindStatusCondition(a.Status.Conditions, controller.ConditionSchemaValid)
	if c == nil || c.Status != "False" || c.Reason != controller.ReasonInvalidSchema {
		t.Fatalf("broken schema should report SchemaValid=False/InvalidSchema: %+v", a.Status.Conditions)
	}
	// the workload is untouched by a bad schema
	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns,
		Name: controller.AdapterDeploymentName("sch-adapter")}, &deploy); err != nil {
		t.Fatalf("an invalid schema must not block the Deployment: %v", err)
	}
	if !apimeta.IsStatusConditionTrue(a.Status.Conditions, controller.ConditionDeployed) {
		t.Fatalf("Deployed must be unaffected: %+v", a.Status.Conditions)
	}
}

// ConfigValid is advisory and appears only when there is a contract to check.
func TestChannelConfigValidLifecycle(t *testing.T) {
	ctx := context.Background()
	chanRec := &controller.ChannelReconciler{Client: k8sClient}
	reconcileChan := func(name string) {
		t.Helper()
		if _, err := chanRec.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
			t.Fatal(err)
		}
	}

	mkAdapter(t, "cv-adapter")
	reconcileAdapter(t, "cv-adapter")
	mkChannel(t, "cv-ok", "cv-adapter") // config: {"chatId":"-100","pollingEnabled":true}

	// nothing declared → no condition at all ("no contract", not "unknown")
	reconcileChan("cv-ok")
	if c := channelCondition(t, "cv-ok", controller.ConditionConfigValid); c != nil {
		t.Fatalf("undeclared schema must leave ConfigValid absent: %+v", c)
	}

	// declaring a schema re-validates CRs that already exist
	setChannelAdapterSchema(t, "cv-adapter", chatSchema)
	reconcileChan("cv-ok")
	c := channelCondition(t, "cv-ok", controller.ConditionConfigValid)
	if c == nil || c.Status != "True" || c.Reason != controller.ReasonSchemaValidated {
		t.Fatalf("conforming config should report ConfigValid=True: %+v", c)
	}

	// a violation names the offending field and leaves serving alone
	violating := &agentopsv1alpha1.Channel{}
	violating.Name, violating.Namespace = "cv-bad", ns
	violating.Spec.Adapter = "cv-adapter"
	violating.Spec.Config = &runtime.RawExtension{Raw: []byte(`{"feedThreadId":"two"}`)}
	if err := k8sClient.Create(ctx, violating); err != nil {
		t.Fatal(err)
	}
	reconcileChan("cv-bad")
	c = channelCondition(t, "cv-bad", controller.ConditionConfigValid)
	if c == nil || c.Status != "False" || c.Reason != controller.ReasonSchemaViolation {
		t.Fatalf("violating config should report ConfigValid=False: %+v", c)
	}
	if !strings.Contains(c.Message, "chatId") && !strings.Contains(c.Message, "feedThreadId") {
		t.Fatalf("message should name the offending field(s): %q", c.Message)
	}
	if served := channelCondition(t, "cv-bad", controller.ConditionServed); served == nil {
		t.Fatal("Served must still be evaluated for a violating channel")
	}

	// an uncompilable schema disables validation rather than reporting failure
	setChannelAdapterSchema(t, "cv-adapter", `{"type":"objekt"}`)
	reconcileChan("cv-ok")
	if c := channelCondition(t, "cv-ok", controller.ConditionConfigValid); c != nil {
		t.Fatalf("uncompilable schema must clear ConfigValid: %+v", c)
	}

	// removing the declaration clears the condition too
	setChannelAdapterSchema(t, "cv-adapter", chatSchema)
	reconcileChan("cv-ok")
	if channelCondition(t, "cv-ok", controller.ConditionConfigValid) == nil {
		t.Fatal("precondition: condition should be present again")
	}
	setChannelAdapterSchema(t, "cv-adapter", "")
	reconcileChan("cv-ok")
	if c := channelCondition(t, "cv-ok", controller.ConditionConfigValid); c != nil {
		t.Fatalf("removing the declaration must clear ConfigValid: %+v", c)
	}
}

// The signal side mirrors it, and a violation must not disturb Served/Wired.
func TestSignalSourceConfigValidIsAdvisory(t *testing.T) {
	ctx := context.Background()
	srcRec := &controller.SignalSourceReconciler{Client: k8sClient}
	reconcileSrc := func(name string) {
		t.Helper()
		if _, err := srcRec.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
			t.Fatal(err)
		}
	}

	mkSignalAdapter(t, "cvs-adapter")
	reconcileSignalAdapter(t, "cvs-adapter")
	mkProfile(t, "prof-cvs")
	mkSignalSource(t, "cvs-src", "cvs-adapter", "")
	mkPipeline(t, "cvs-pipe", []string{"cvs-src"}, nil, "prof-cvs")
	reconcilePipeline(t, "cvs-pipe")

	var a agentopsv1alpha1.SignalAdapter
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "cvs-adapter"}, &a); err != nil {
		t.Fatal(err)
	}
	a.Spec.ConfigSchema = &runtime.RawExtension{Raw: []byte(
		`{"type":"object","properties":{"schedule":{"type":"string"}},"required":["schedule"]}`)}
	if err := k8sClient.Update(ctx, &a); err != nil {
		t.Fatal(err)
	}

	reconcileSrc("cvs-src")
	var src agentopsv1alpha1.SignalSource
	_ = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "cvs-src"}, &src)
	c := apimeta.FindStatusCondition(src.Status.Conditions, controller.ConditionConfigValid)
	if c == nil || c.Status != "False" || !strings.Contains(c.Message, "schedule") {
		t.Fatalf("source with no config should violate a required field: %+v", c)
	}
	// advisory: wiring is untouched by the violation
	if !apimeta.IsStatusConditionTrue(src.Status.Conditions, controller.ConditionWired) {
		t.Fatalf("ConfigValid=False must not affect Wired: %+v", src.Status.Conditions)
	}
}
