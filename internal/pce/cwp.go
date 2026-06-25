package pce

import (
	"context"
	"net/http"
)

// CWPLabel is a label assignment on a Container Workload Profile. Set exactly
// one of Assignment (a fixed label) or Restriction (an allow-list).
type CWPLabel struct {
	Key         string     `json:"key"`
	Assignment  *LabelRef  `json:"assignment,omitempty"`
	Restriction []LabelRef `json:"restriction,omitempty"`
}

// ContainerWorkloadProfile is a per-namespace policy/label profile in the PCE.
// Kubelink creates one per namespace; this operator updates them.
type ContainerWorkloadProfile struct {
	Href            string     `json:"href,omitempty"`
	Name            string     `json:"name,omitempty"`
	Namespace       string     `json:"namespace,omitempty"`
	Managed         bool       `json:"managed"`
	EnforcementMode string     `json:"enforcement_mode,omitempty"`
	VisibilityLevel string     `json:"visibility_level,omitempty"`
	Labels          []CWPLabel `json:"labels,omitempty"`
}

// CWPUpdate is the body of a CWP update; only set fields are changed.
// Labels must NOT use omitempty: an explicit empty slice serializes as
// "labels":[] which instructs the PCE to clear all label assignments.
type CWPUpdate struct {
	Managed         *bool      `json:"managed,omitempty"`
	EnforcementMode string     `json:"enforcement_mode,omitempty"`
	Labels          []CWPLabel `json:"labels"`
}

// ListContainerWorkloadProfiles lists the CWPs for a container cluster.
func (c *Client) ListContainerWorkloadProfiles(ctx context.Context, clusterID string) ([]ContainerWorkloadProfile, error) {
	var out []ContainerWorkloadProfile
	path := c.orgPath("/container_clusters/" + clusterID + "/container_workload_profiles")
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateContainerWorkloadProfile PUTs an update to the CWP identified by its
// href (an href like /orgs/N/container_clusters/C/container_workload_profiles/P).
func (c *Client) UpdateContainerWorkloadProfile(ctx context.Context, profileHref string, update CWPUpdate) error {
	return c.do(ctx, http.MethodPut, profileHref, update, nil)
}
