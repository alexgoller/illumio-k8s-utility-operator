package pce

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// Owner carries the ownership tags stamped on PCE objects the operator creates.
type Owner struct {
	DataSet   string
	Reference string
}

// Label is an Illumio label object.
type Label struct {
	Href                  string `json:"href,omitempty"`
	Key                   string `json:"key"`
	Value                 string `json:"value"`
	ExternalDataSet       string `json:"external_data_set,omitempty"`
	ExternalDataReference string `json:"external_data_reference,omitempty"`
}

// FindLabel returns the label with the exact key/value, or ErrLabelNotFound.
func (c *Client) FindLabel(ctx context.Context, key, value string) (*Label, error) {
	q := url.Values{}
	q.Set("key", key)
	q.Set("value", value)
	var labels []Label
	if err := c.do(ctx, http.MethodGet, c.orgPath("/labels")+"?"+q.Encode(), nil, &labels); err != nil {
		return nil, err
	}
	for i := range labels {
		if labels[i].Key == key && labels[i].Value == value {
			return &labels[i], nil
		}
	}
	return nil, ErrLabelNotFound
}

// CreateLabel creates a new label and returns it (with its assigned href).
func (c *Client) CreateLabel(ctx context.Context, l Label) (*Label, error) {
	var created Label
	if err := c.do(ctx, http.MethodPost, c.orgPath("/labels"), l, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// EnsureLabel returns the existing label for key/value, or creates it stamped
// with the given ownership tags.
func (c *Client) EnsureLabel(ctx context.Context, key, value string, owner Owner) (*Label, error) {
	existing, err := c.FindLabel(ctx, key, value)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrLabelNotFound) {
		return nil, err
	}
	return c.CreateLabel(ctx, Label{
		Key:                   key,
		Value:                 value,
		ExternalDataSet:       owner.DataSet,
		ExternalDataReference: owner.Reference,
	})
}
