package ingest

import "testing"

func TestAdmitsEmptyRequirementsAdmitsEverything(t *testing.T) {
	if !Admits(nil, nil) {
		t.Fatal("nil requirements must admit a signal with no labels")
	}
	if !Admits([]FilterRequirement{}, map[string]string{"team": "payments"}) {
		t.Fatal("empty requirements must admit any labels")
	}
}

func TestAdmitsIn(t *testing.T) {
	reqs := []FilterRequirement{{Key: "team", Operator: "In", Values: []string{"payments", "billing"}}}
	if !Admits(reqs, map[string]string{"team": "payments"}) {
		t.Fatal("want admitted: team=payments is in the list")
	}
	if Admits(reqs, map[string]string{"team": "search"}) {
		t.Fatal("want dropped: team=search is not in the list")
	}
	if Admits(reqs, map[string]string{}) {
		t.Fatal("want dropped: key absent entirely fails In")
	}
}

func TestAdmitsNotIn(t *testing.T) {
	reqs := []FilterRequirement{{Key: "team", Operator: "NotIn", Values: []string{"test"}}}
	if !Admits(reqs, map[string]string{"team": "payments"}) {
		t.Fatal("want admitted: team=payments is not excluded")
	}
	if Admits(reqs, map[string]string{"team": "test"}) {
		t.Fatal("want dropped: team=test is excluded")
	}
}

func TestAdmitsExists(t *testing.T) {
	reqs := []FilterRequirement{{Key: "team", Operator: "Exists"}}
	if !Admits(reqs, map[string]string{"team": "anything"}) {
		t.Fatal("want admitted: key present")
	}
	if Admits(reqs, map[string]string{"other": "x"}) {
		t.Fatal("want dropped: key absent")
	}
}

func TestAdmitsDoesNotExist(t *testing.T) {
	reqs := []FilterRequirement{{Key: "team", Operator: "DoesNotExist"}}
	if !Admits(reqs, map[string]string{"other": "x"}) {
		t.Fatal("want admitted: key absent")
	}
	if Admits(reqs, map[string]string{"team": "anything"}) {
		t.Fatal("want dropped: key present")
	}
}

// Every requirement in the list must be satisfied — an AND, not an OR.
func TestAdmitsRequiresEveryRequirement(t *testing.T) {
	reqs := []FilterRequirement{
		{Key: "team", Operator: "In", Values: []string{"payments"}},
		{Key: "env", Operator: "In", Values: []string{"prod"}},
	}
	if !Admits(reqs, map[string]string{"team": "payments", "env": "prod"}) {
		t.Fatal("want admitted: both requirements satisfied")
	}
	if Admits(reqs, map[string]string{"team": "payments", "env": "staging"}) {
		t.Fatal("want dropped: one requirement failed")
	}
}

// A value-arity-malformed requirement (right operator, wrong Values shape)
// fails closed: the signal it gates is never admitted, design's "never
// open" guarantee.
func TestAdmitsFailsClosedOnMalformedRequirement(t *testing.T) {
	inNoValues := []FilterRequirement{{Key: "team", Operator: "In", Values: nil}}
	if Admits(inNoValues, map[string]string{"team": "payments"}) {
		t.Fatal("In with no values must fail closed, not admit everything")
	}

	existsWithValues := []FilterRequirement{{Key: "team", Operator: "Exists", Values: []string{"payments"}}}
	if Admits(existsWithValues, map[string]string{"team": "payments"}) {
		t.Fatal("Exists with values must fail closed, not admit everything")
	}

	unknownOperator := []FilterRequirement{{Key: "team", Operator: "Frobnicate", Values: []string{"payments"}}}
	if Admits(unknownOperator, map[string]string{"team": "payments"}) {
		t.Fatal("an unknown operator must fail closed, not admit everything")
	}
}
