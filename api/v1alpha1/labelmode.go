package v1alpha1

// Unknown-label handling modes. See the policy-model roadmap design doc
// (docs/superpowers/specs/2026-06-30-policy-model-roadmap-design.md, Track 1).
const (
	UnknownLabelStrict = "strict" // reject the whole CR (default)
	UnknownLabelSkip   = "skip"   // omit the unknown actor/rule, report it
	UnknownLabelCreate = "create" // mint the label (standard keys only), then use it
)

// AnnotationUnknownLabelMode overrides the unknown-label mode on a namespace or a policy CR.
const AnnotationUnknownLabelMode = "microsegment.io/unknown-label-mode"

func validUnknownLabelMode(m string) bool {
	return m == UnknownLabelStrict || m == UnknownLabelSkip || m == UnknownLabelCreate
}

// ResolveUnknownLabelMode picks the effective mode, most-specific wins:
// CR annotation > namespace annotation > ClusterProfile default > strict.
// setBy is one of "cr", "namespace", "clusterprofile", "default".
func ResolveUnknownLabelMode(cpDefault string, nsAnnotations, crAnnotations map[string]string) (mode, setBy string) {
	if v := crAnnotations[AnnotationUnknownLabelMode]; validUnknownLabelMode(v) {
		return v, "cr"
	}
	if v := nsAnnotations[AnnotationUnknownLabelMode]; validUnknownLabelMode(v) {
		return v, "namespace"
	}
	if validUnknownLabelMode(cpDefault) {
		return cpDefault, "clusterprofile"
	}
	return UnknownLabelStrict, "default"
}
