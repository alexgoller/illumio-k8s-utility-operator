package v1alpha1

import "testing"

func TestResolveUnknownLabelMode(t *testing.T) {
	cases := []struct {
		name, cpDefault  string
		ns, cr           map[string]string
		wantMode, wantBy string
	}{
		{"default empty -> strict", "", nil, nil, UnknownLabelStrict, "default"},
		{"clusterprofile default", "skip", nil, nil, UnknownLabelSkip, "clusterprofile"},
		{"namespace overrides cp", "skip", map[string]string{AnnotationUnknownLabelMode: "create"}, nil, UnknownLabelCreate, "namespace"},
		{"cr overrides namespace", "skip", map[string]string{AnnotationUnknownLabelMode: "create"}, map[string]string{AnnotationUnknownLabelMode: "strict"}, UnknownLabelStrict, "cr"},
		{"invalid value ignored -> falls through", "", nil, map[string]string{AnnotationUnknownLabelMode: "bogus"}, UnknownLabelStrict, "default"},
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
