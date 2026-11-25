package main

import (
	"context"
	"testing"
)

func TestPolicyFailsClosed(t *testing.T) {
	for _, candidate := range []candidate{{Answer: "unsupported guess"}, {Answer: "guessed", Abstained: true}, {Answer: "bad source", Citations: []struct {
		ID string `json:"chunk_id"`
	}{{ID: "unknown"}}}} {
		result, err := new(app).resolve(context.Background(), "unused", candidate)
		if err != nil || !result.Abstained || len(result.Citations) > 0 {
			t.Fatal("Invalid candidate did not abstain")
		}
	}
}
