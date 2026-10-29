// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"context"
	"testing"
)

type nilSafeEmbedder struct{}

func (*nilSafeEmbedder) Model() string { return "test" }
func (*nilSafeEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return [][]float64{{1, 2, 3}}, nil
}

func TestUsableEmbedderRejectsTypedNil(t *testing.T) {
	var typedNil *nilSafeEmbedder
	if usableEmbedder(typedNil) {
		t.Fatal("typed nil embedder must be rejected")
	}
	if !usableEmbedder(&nilSafeEmbedder{}) {
		t.Fatal("non-nil embedder must be accepted")
	}
	if usableEmbedder(nil) {
		t.Fatal("nil interface must be rejected")
	}
}
