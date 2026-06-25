package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// PCEPinger is the subset of the PCE client the connection controller needs.
type PCEPinger interface {
	Ping(ctx context.Context) error
}

// ClientFactory builds a PCEPinger from a Config. Injectable for tests.
type ClientFactory func(cfg pce.Config) PCEPinger

// DefaultClientFactory wraps the real PCE client.
func DefaultClientFactory(cfg pce.Config) PCEPinger {
	return pce.NewClient(cfg)
}
