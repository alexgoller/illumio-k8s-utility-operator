package pce

import (
	"context"
	"net/http"
	"strings"
)

// ContainerCluster is the PCE object representing a Kubernetes cluster.
type ContainerCluster struct {
	Href        string `json:"href,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// ContainerClusterToken is returned ONLY at creation. Persist immediately.
	ContainerClusterToken string `json:"container_cluster_token,omitempty"`
	ExternalDataSet       string `json:"external_data_set,omitempty"`
	ExternalDataReference string `json:"external_data_reference,omitempty"`
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
// one-time container_cluster_token. Ownership tags are stamped on creation.
func (c *Client) CreateContainerCluster(ctx context.Context, name, description string, owner Owner) (*ContainerCluster, error) {
	body := ContainerCluster{
		Name:                  name,
		Description:           description,
		ExternalDataSet:       owner.DataSet,
		ExternalDataReference: owner.Reference,
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
