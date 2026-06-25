package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// OnboardingClient is the subset of the PCE client the ClusterProfile
// controller needs. The real *pce.Client satisfies it.
type OnboardingClient interface {
	FindContainerClusterByName(ctx context.Context, name string) (*pce.ContainerCluster, error)
	CreateContainerCluster(ctx context.Context, name, description string, owner pce.Owner) (*pce.ContainerCluster, error)
	FindPairingProfileByName(ctx context.Context, name string) (*pce.PairingProfile, error)
	CreatePairingProfile(ctx context.Context, pp pce.PairingProfile) (*pce.PairingProfile, error)
	GeneratePairingKey(ctx context.Context, profileHref string) (string, error)
	EnsureLabel(ctx context.Context, key, value string, owner pce.Owner) (*pce.Label, error)
}

// Compile-time assertion: *pce.Client must implement OnboardingClient.
var _ OnboardingClient = (*pce.Client)(nil)

// OnboardingClientFactory builds an OnboardingClient from a Config (injectable for tests).
type OnboardingClientFactory func(cfg pce.Config) OnboardingClient

// DefaultOnboardingClientFactory wraps the real PCE client.
func DefaultOnboardingClientFactory(cfg pce.Config) OnboardingClient {
	return pce.NewClient(cfg)
}
