package controller

import (
	"fmt"
	"slices"
	"strings"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// mapRules converts found rules into RuleSummaries, flags operator-owned vs
// external (by comparing the rule's ruleset external_data_set to the operator's
// data set eds), and caps the list for etcd safety. Returns the summaries plus
// the true owned/external counts (over ALL found rules, not just the capped list)
// and whether the list was truncated.
func mapRules(found []pce.FoundRule, eds string, capN int) (rules []microv1.RuleSummary, owned, external int, truncated bool) {
	all := make([]microv1.RuleSummary, 0, len(found))
	for i := range found {
		f := &found[i]
		ownedBy := microv1.RuleOwnedByExternal
		if eds != "" && f.RulesetExternalDataSet == eds {
			ownedBy = microv1.RuleOwnedByOperator
			owned++
		} else {
			external++
		}
		all = append(all, microv1.RuleSummary{
			Href:        f.Href,
			RulesetName: f.RulesetName,
			OwnedBy:     ownedBy,
			Type:        f.Type,
			Enabled:     f.Enabled,
			Consumers:   renderActors(f.Consumers),
			Services:    renderServices(f.Services),
		})
	}
	// Stable order: owned first, then by ruleset name, then href.
	slices.SortFunc(all, func(a, b microv1.RuleSummary) int {
		if a.OwnedBy != b.OwnedBy {
			if a.OwnedBy == microv1.RuleOwnedByOperator {
				return -1
			}
			return 1
		}
		if a.RulesetName != b.RulesetName {
			return strings.Compare(a.RulesetName, b.RulesetName)
		}
		return strings.Compare(a.Href, b.Href)
	})
	if capN > 0 && len(all) > capN {
		return all[:capN], owned, external, true
	}
	return all, owned, external, false
}

// renderActors turns rule actors into display strings.
func renderActors(actors []pce.Actor) []string {
	out := make([]string, 0, len(actors))
	for i := range actors {
		a := &actors[i]
		switch {
		case a.Actors == pce.ActorAllWorkloads:
			out = append(out, pce.ActorAllWorkloads)
		case a.Label != nil && a.Label.Href != "":
			out = append(out, "label:"+lastPathSegmentPub(a.Label.Href))
		}
	}
	return out
}

// renderServices turns ingress services into "<port>/<proto>" or "All Services".
func renderServices(svcs []pce.IngressService) []string {
	if len(svcs) == 0 {
		return nil
	}
	out := make([]string, 0, len(svcs))
	for i := range svcs {
		s := &svcs[i]
		if s.Href != "" && s.Port == 0 && s.Proto == 0 {
			out = append(out, "All Services")
			continue
		}
		out = append(out, fmt.Sprintf("%d/%s", s.Port, protoName(s.Proto)))
	}
	return out
}

func lastPathSegmentPub(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}
