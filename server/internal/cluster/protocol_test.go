package cluster

import "testing"

func TestReadOnlyRequestClassification(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		readOnly     bool
	}{
		{"GET", "/api/containers", true},
		{"POST", "/api/images/check-update", true},
		{"POST", "/api/images/check-update?force=true", true},
		{"POST", "/api/images/check-update-extra", false},
		{"DELETE", "/api/images/check-update", false},
		{"POST", "/api/images/pull", false},
		{"POST", "/api/images/prune", false},
		{"POST", "/api/containers/demo/update", false},
		{"PUT", "/api/projects/demo", false},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if got := isReadOnlyRequest(&Request{Method: tc.method, Path: tc.path}); got != tc.readOnly {
				t.Fatalf("readOnly=%v, want %v", got, tc.readOnly)
			}
		})
	}
	if isReadOnlyRequest(nil) {
		t.Fatal("nil request accepted")
	}
}
