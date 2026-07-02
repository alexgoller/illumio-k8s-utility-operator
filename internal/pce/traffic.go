package pce

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Policy decision values (both reported policy_decision and draft_policy_decision).
const (
	DecisionAllowed            = "allowed"
	DecisionPotentiallyBlocked = "potentially_blocked"
	DecisionBlocked            = "blocked"
	DecisionUnknown            = "unknown"
)

// TrafficQuery selects observed flows for a scope + window. Direction is
// expressed by which side carries the scope label hrefs: set
// DestinationLabelHrefs for inbound-to-app, SourceLabelHrefs for egress-from-app.
type TrafficQuery struct {
	QueryName             string
	SourceLabelHrefs      []string
	DestinationLabelHrefs []string
	From, To              time.Time
	MaxResults            int
}

// TrafficFlow is one aggregated observed flow with its policy decisions.
type TrafficFlow struct {
	SrcLabels           map[string]string
	DstLabels           map[string]string
	SrcIP, DstIP        string
	Port                int
	Protocol            int // 6=TCP, 17=UDP
	PolicyDecision      string
	DraftPolicyDecision string
	Connections         int
	LastDetected        time.Time
}

// --- wire types (Illumio Explorer async traffic query) ---

type trafficActor struct {
	Label *LabelRef `json:"label,omitempty"`
}

type trafficActorSet struct {
	Include [][]trafficActor `json:"include"`
	Exclude []trafficActor   `json:"exclude"`
}

type trafficQueryBody struct {
	QueryName    string          `json:"query_name"`
	StartDate    string          `json:"start_date"`
	EndDate      string          `json:"end_date"`
	Sources      trafficActorSet `json:"sources"`
	Destinations trafficActorSet `json:"destinations"`
	Services     struct {
		Include []any `json:"include"`
		Exclude []any `json:"exclude"`
	} `json:"services"`
	PolicyDecisions []string `json:"policy_decisions"`
	MaxResults      int      `json:"max_results"`
}

type asyncQueryStatus struct {
	Href   string `json:"href"`
	Status string `json:"status"`
}

type wireEndpoint struct {
	IP       string `json:"ip"`
	Workload *struct {
		Labels []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"labels"`
	} `json:"workload"`
}

type wireFlow struct {
	Src     wireEndpoint `json:"src"`
	Dst     wireEndpoint `json:"dst"`
	Service struct {
		Port  int `json:"port"`
		Proto int `json:"proto"`
	} `json:"service"`
	NumConnections      int    `json:"num_connections"`
	PolicyDecision      string `json:"policy_decision"`
	DraftPolicyDecision string `json:"draft_policy_decision"`
	TimestampRange      struct {
		LastDetected string `json:"last_detected"`
	} `json:"timestamp_range"`
}

func labelActors(hrefs []string) [][]trafficActor {
	if len(hrefs) == 0 {
		return [][]trafficActor{}
	}
	inner := make([]trafficActor, 0, len(hrefs))
	for _, h := range hrefs {
		inner = append(inner, trafficActor{Label: &LabelRef{Href: h}})
	}
	return [][]trafficActor{inner}
}

func endpointLabels(e wireEndpoint) map[string]string {
	if e.Workload == nil || len(e.Workload.Labels) == 0 {
		return nil
	}
	m := make(map[string]string, len(e.Workload.Labels))
	for _, l := range e.Workload.Labels {
		m[l.Key] = l.Value
	}
	return m
}

// QueryTraffic runs the async Explorer traffic query (POST → poll → download)
// and returns the observed flows. truncated is true if the result hit MaxResults.
//
// NOTE: the exact download path and result envelope vary by PCE version; this
// targets the documented Core 24.x shape and may need a small live-tuning pass
// (see docs/superpowers/specs/2026-07-02-preflight-traffic-api-spike.md).
func (c *Client) QueryTraffic(ctx context.Context, q TrafficQuery) ([]TrafficFlow, bool, error) {
	max := q.MaxResults
	if max <= 0 {
		max = 10000
	}
	body := trafficQueryBody{
		QueryName:       q.QueryName,
		StartDate:       q.From.UTC().Format(time.RFC3339),
		EndDate:         q.To.UTC().Format(time.RFC3339),
		Sources:         trafficActorSet{Include: labelActors(q.SourceLabelHrefs), Exclude: []trafficActor{}},
		Destinations:    trafficActorSet{Include: labelActors(q.DestinationLabelHrefs), Exclude: []trafficActor{}},
		PolicyDecisions: []string{},
		MaxResults:      max,
	}
	body.Services.Include = []any{}
	body.Services.Exclude = []any{}

	var created asyncQueryStatus
	if err := c.do(ctx, http.MethodPost, c.orgPath("/traffic_flows/async_queries"), body, &created); err != nil {
		return nil, false, fmt.Errorf("create traffic query: %w", err)
	}
	uuid := lastPathSegment(created.Href)
	if uuid == "" {
		return nil, false, fmt.Errorf("traffic query returned no href")
	}

	// Poll until completed (bounded — on-request, so a short synchronous wait).
	statusPath := c.orgPath("/traffic_flows/async_queries/" + uuid)
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		var st asyncQueryStatus
		if err := c.do(ctx, http.MethodGet, statusPath, nil, &st); err != nil {
			return nil, false, fmt.Errorf("poll traffic query: %w", err)
		}
		if st.Status == "completed" || st.Status == "done" {
			break
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-deadline.C:
			return nil, false, fmt.Errorf("traffic query %s did not complete in time", uuid)
		case <-time.After(2 * time.Second):
		}
	}

	var flows []wireFlow
	if err := c.do(ctx, http.MethodGet, c.orgPath("/traffic_flows/async_queries/"+uuid+"/download"), nil, &flows); err != nil {
		return nil, false, fmt.Errorf("download traffic query: %w", err)
	}

	out := make([]TrafficFlow, 0, len(flows))
	for i := range flows {
		f := &flows[i]
		last, _ := time.Parse(time.RFC3339, f.TimestampRange.LastDetected)
		out = append(out, TrafficFlow{
			SrcLabels:           endpointLabels(f.Src),
			DstLabels:           endpointLabels(f.Dst),
			SrcIP:               f.Src.IP,
			DstIP:               f.Dst.IP,
			Port:                f.Service.Port,
			Protocol:            f.Service.Proto,
			PolicyDecision:      f.PolicyDecision,
			DraftPolicyDecision: f.DraftPolicyDecision,
			Connections:         f.NumConnections,
			LastDetected:        last,
		})
	}
	return out, len(out) >= max, nil
}

func lastPathSegment(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return s[i+1:]
		}
	}
	return s
}
