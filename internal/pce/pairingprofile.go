package pce

import (
	"context"
	"net/http"
)

// LabelRef references an Illumio label by href.
type LabelRef struct {
	Href string `json:"href"`
}

// PairingProfile is the PCE object that issues C-VEN pairing keys and assigns
// labels to the nodes paired with the generated key.
type PairingProfile struct {
	Href            string `json:"href,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Enabled         bool   `json:"enabled"`
	EnforcementMode string `json:"enforcement_mode,omitempty"`
	VisibilityLevel string `json:"visibility_level,omitempty"`
	// AllowedUsesPerKey and KeyLifespan accept the literal string "unlimited".
	AllowedUsesPerKey     string     `json:"allowed_uses_per_key,omitempty"`
	KeyLifespan           string     `json:"key_lifespan,omitempty"`
	Labels                []LabelRef `json:"labels,omitempty"`
	ExternalDataSet       string     `json:"external_data_set,omitempty"`
	ExternalDataReference string     `json:"external_data_reference,omitempty"`
}

// FindPairingProfileByName returns the profile with the exact name, or (nil, nil).
func (c *Client) FindPairingProfileByName(ctx context.Context, name string) (*PairingProfile, error) {
	var profiles []PairingProfile
	if err := c.do(ctx, http.MethodGet, c.orgPath("/pairing_profiles"), nil, &profiles); err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].Name == name {
			return &profiles[i], nil
		}
	}
	return nil, nil
}

// CreatePairingProfile creates a pairing profile and returns it (with its href).
func (c *Client) CreatePairingProfile(ctx context.Context, pp PairingProfile) (*PairingProfile, error) {
	var created PairingProfile
	if err := c.do(ctx, http.MethodPost, c.orgPath("/pairing_profiles"), pp, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// pairingKeyResponse models the generate-pairing-key response.
type pairingKeyResponse struct {
	ActivationCode string `json:"activation_code"`
}

// GeneratePairingKey generates a new pairing key (activation code) for the
// profile identified by profileHref (an href like /orgs/7/pairing_profiles/5).
func (c *Client) GeneratePairingKey(ctx context.Context, profileHref string) (string, error) {
	var resp pairingKeyResponse
	if err := c.do(ctx, http.MethodPost, profileHref+"/pairing_key", struct{}{}, &resp); err != nil {
		return "", err
	}
	return resp.ActivationCode, nil
}
