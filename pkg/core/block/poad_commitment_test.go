package block

import (
	"bytes"
	"testing"
)

func TestHeaderHashCommitsToPoADRoot(t *testing.T) {
	base := Header{StateRoot: []byte{1, 2, 3}}
	firstHash, err := base.Hash()
	if err != nil {
		t.Fatal(err)
	}

	withPoAD := base
	withPoAD.PoADRoot = []byte{4, 5, 6}
	secondHash, err := withPoAD.Hash()
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(firstHash, secondHash) {
		t.Fatal("expected PoADRoot to affect the block header hash")
	}
}
