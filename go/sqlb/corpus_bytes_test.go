package sqlb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand/v2"
	"testing"
)

// TestCorpusBytesAreUnchanged pins the exact bytes of the generated corpus for
// the six seeds README.md's "150,000 cases, zero mismatches" claim refers to.
//
// It exists because the differential runner was extracted into
// github.com/trutohq/differential (Truto's H-1), and an extraction that changes
// the corpus is not an extraction — every recorded seed would silently start
// meaning a different set of cases, and the zero-mismatch history would be
// about corpora nobody had ever run. The hashes below were taken from the tree
// BEFORE the extraction.
//
// If a deliberate change to the generator moves these, re-record them from a
// run and say so in the commit. Do not adjust them to make a test pass.
func TestCorpusBytesAreUnchanged(t *testing.T) {
	// seed -> sha256 of json.Marshal(generateCases(...)) at 25,000 cases,
	// stream 0x1979.
	want := map[uint64]string{
		0x5150:   "140120663306b1ffb2b2435ef7e08a3e518a97ad2da36cd217cbd20864ca4831",
		1:        "0e68e44519432419db9cd6cac07baa4f3dc8e329d0b71318e2d6313339233f05",
		2:        "de6669f48950ffb1e7975ada9673ddbd56cb5797f7990da2a3ad2c38bb8106f6",
		3:        "14a08593953353c26cdabe5db955d2e619cd22aa4ac915fa17f58f4f04a288d9",
		7:        "a2bc70b98601fc8827e2f4a375e6dbbee1f16ed9b38badb7551b7827206898da",
		20260912: "622d55fed7f22a786d40a4242e29de820c881685fea37239f0eda0b7ddbc7eea",
	}
	for seed, sum := range want {
		cases := generateCases(rand.New(rand.NewPCG(seed, 0x1979)), 25000)
		b, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		got := sha256.Sum256(b)
		if hex.EncodeToString(got[:]) != sum {
			t.Errorf("seed %d: corpus is %s, was %s — the extraction changed the corpus",
				seed, hex.EncodeToString(got[:]), sum)
		}
	}
}
