package v1alpha1

import "testing"

func TestResolveUnknownLabelMode(t *testing.T) {
	cases := []struct {
		name, cpDefault  string
		ns, cr           map[string]string
		wantMode, wantBy string
	}{
		{"default empty -> strict", "", nil, nil, UnknownLabelStrict, SetBySourceDefault},
		{"clusterprofile default", UnknownLabelSkip, nil, nil, UnknownLabelSkip, SetBySourceClusterProfile},
		{"namespace overrides cp", UnknownLabelSkip, map[string]string{AnnotationUnknownLabelMode: UnknownLabelCreate}, nil, UnknownLabelCreate, SetBySourceNamespace},
		{"cr overrides namespace", UnknownLabelSkip, map[string]string{AnnotationUnknownLabelMode: UnknownLabelCreate}, map[string]string{AnnotationUnknownLabelMode: UnknownLabelStrict}, UnknownLabelStrict, SetBySourceCR},
		{"invalid value ignored -> falls through", "", nil, map[string]string{AnnotationUnknownLabelMode: "bogus"}, UnknownLabelStrict, SetBySourceDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, by := ResolveUnknownLabelMode(tc.cpDefault, tc.ns, tc.cr)
			if mode != tc.wantMode || by != tc.wantBy {
				t.Fatalf("got %q/%q, want %q/%q", mode, by, tc.wantMode, tc.wantBy)
			}
		})
	}
}
