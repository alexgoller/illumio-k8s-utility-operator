package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// InsightClient is the subset of the PCE client the PolicyInsight controller
// needs: resolve scope labels to hrefs, and run observed-traffic queries.
type InsightClient interface {
	FindLabel(ctx context.Context, key, value string) (*pce.Label, error)
	QueryTraffic(ctx context.Context, q pce.TrafficQuery) ([]pce.TrafficFlow, bool, error)
}

var _ InsightClient = (*pce.Client)(nil)

// InsightClientFactory builds an InsightClient (injectable for tests).
type InsightClientFactory func(cfg pce.Config) InsightClient

// DefaultInsightClientFactory wraps the real PCE client.
func DefaultInsightClientFactory(cfg pce.Config) InsightClient { return pce.NewClient(cfg) }
