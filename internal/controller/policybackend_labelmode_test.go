package controller

import (
	"context"
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// stubLabelClient embeds the suite fake and overrides FindLabel so value
// "missing" is unknown and everything else resolves.
type stubLabelClient struct{ fakePolicyClient }

func (stubLabelClient) FindLabel(_ context.Context, key, value string) (*pce.Label, error) {
	if value == "missing" {
		return nil, pce.ErrLabelNotFound
	}
	return &pce.Label{Href: "/h/" + key + "-" + value, Key: key, Value: value}, nil
}

func TestResolveAllows_Modes(t *testing.T) {
	allows := []CompiledAllow{
		{From: map[string]string{"app": "known"}},
		{From: map[string]string{"app": "missing"}},
	}
	ctx := context.Background()
	c := stubLabelClient{}

	// strict: unknown -> not ok
	if _, _, _, _, _, ok, _ := resolveAllows(ctx, allows, c, microv1.UnknownLabelStrict, pce.Owner{}); ok {
		t.Fatal("strict must reject on unknown label")
	}
	// skip: ok, the unknown allow is dropped, deferred lists it
	res, deferred, _, _, _, ok, _ := resolveAllows(ctx, allows, c, microv1.UnknownLabelSkip, pce.Owner{})
	if !ok || len(res) != 1 || len(deferred) != 1 || deferred[0] != "app=missing" {
		t.Fatalf("skip: res=%d deferred=%v ok=%v", len(res), deferred, ok)
	}
	// create: ok, the missing label is minted, created lists it, both allows kept
	res, _, created, _, _, ok, _ := resolveAllows(ctx, allows, c, microv1.UnknownLabelCreate, pce.Owner{})
	if !ok || len(res) != 2 || len(created) != 1 || created[0] != "app=missing" {
		t.Fatalf("create: res=%d created=%v ok=%v", len(res), created, ok)
	}
	// create with a non-standard key -> reject
	bad := []CompiledAllow{{From: map[string]string{"team": "missing"}}}
	if _, _, _, _, _, ok, _ := resolveAllows(ctx, bad, c, microv1.UnknownLabelCreate, pce.Owner{}); ok {
		t.Fatal("create must reject auto-creating a non-standard key")
	}
}
