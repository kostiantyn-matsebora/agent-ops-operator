//go:build system

package system

import (
	"reflect"
	"strings"
	"testing"
)

func TestBaseImagesAreTheExternalOnesOnly(t *testing.T) {
	df := `# comment
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
RUN true
FROM build AS test
FROM gcr.io/distroless/static:nonroot
FROM scratch
FROM ${BASE}
FROM node:22-alpine AS ui
FROM golang:1.27
`
	got := baseImages(df)
	want := []string{"golang:1.27", "gcr.io/distroless/static:nonroot", "node:22-alpine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDerivedDockerfileNeedsNoShellAndPointsClientsAtTheBundle(t *testing.T) {
	df := derivedDockerfile("node:22-alpine")
	if strings.Contains(df, "RUN") {
		t.Fatalf("a RUN would fail on a distroless base: %s", df)
	}
	for _, v := range []string{"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "GIT_SSL_CAINFO", buildCALabel} {
		if !strings.Contains(df, v) {
			t.Errorf("missing %s in %s", v, df)
		}
	}
	if !strings.HasPrefix(df, "FROM node:22-alpine\n") {
		t.Errorf("the base must be the original reference: %s", df)
	}
}

func TestNothingRunsWithoutTheVariable(t *testing.T) {
	t.Setenv(buildCAEnv, "")
	if err := prepareBuildCA(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}
