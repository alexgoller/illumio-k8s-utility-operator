package pce

import (
	"context"
	"net/http"
	"strings"
)

// ContainerCluster is the PCE object representing a Kubernetes cluster.
//
// Unlike most PCE objects, the container_clusters POST/PUT schemas do NOT permit
// external_data_set / external_data_reference (the PCE rejects them with a 406
// input_validation_error). Container clusters are therefore owned-by-name: the
// operator finds its cluster with FindContainerClusterByName rather than by an
// ownership tag.
type ContainerCluster struct {
	Href        string `json:"href,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// ContainerClusterToken is returned ONLY at creation. Persist immediately.
	ContainerClusterToken string `json:"container_cluster_token,omitempty"`
}

// ListContainerClusters returns all container clusters in the org.
func (c *Client) ListContainerClusters(ctx context.Context) ([]ContainerCluster, error) {
	var out []ContainerCluster
	if err := c.do(ctx, http.MethodGet, c.orgPath("/container_clusters"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// FindContainerClusterByName returns the cluster with the exact name, or (nil, nil).
func (c *Client) FindContainerClusterByName(ctx context.Context, name string) (*ContainerCluster, error) {
	clusters, err := c.ListContainerClusters(ctx)
	if err != nil {
		return nil, err
	}
	for i := range clusters {
		if clusters[i].Name == name {
			return &clusters[i], nil
		}
	}
	return nil, nil
}

// CreateContainerCluster creates a cluster and returns it, including the
// one-time container_cluster_token. No ownership tags are sent: the PCE
// container_clusters POST schema rejects external_data_* (406). The cluster is
// identified by name on subsequent reconciles.
func (c *Client) CreateContainerCluster(ctx context.Context, name, description string) (*ContainerCluster, error) {
	body := ContainerCluster{
		Name:        name,
		Description: description,
	}
	var created ContainerCluster
	if err := c.do(ctx, http.MethodPost, c.orgPath("/container_clusters"), body, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// ContainerClusterUUID extracts the cluster UUID (last path segment) from an href.
func ContainerClusterUUID(href string) string {
	if i := strings.LastIndex(href, "/"); i >= 0 {
		return href[i+1:]
	}
	return href
}
