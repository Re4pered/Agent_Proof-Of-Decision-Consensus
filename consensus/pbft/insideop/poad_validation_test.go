package insideop

import (
	"errors"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
)

type fakePoADBlockValidator struct {
	calls int
	err   error
}

func (v *fakePoADBlockValidator) ValidatePoADBlock(_ *block.Block) error {
	v.calls++

	return v.err
}

func TestValidatePoADProposalAllowsLegacyBlockWhenOptional(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	if err := validatePoADProposal(b, nil, false); err != nil {
		t.Fatalf("expected legacy block to be allowed when PoAD is optional: %v", err)
	}
}

func TestValidatePoADProposalRequiresMetadataWhenConfigured(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.TxBlockType}, block.Body{}, block.MigrationOpt{})
	if err := validatePoADProposal(b, &fakePoADBlockValidator{}, true); err == nil {
		t.Fatal("expected transaction proposal without PoAD metadata to be rejected")
	}
}

func TestValidatePoADProposalRequiresValidatorForPresentMetadata(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, PoADRoot: []byte{1}},
		block.Body{},
		block.MigrationOpt{},
	)
	b.PoADData = []byte{2}

	if err := validatePoADProposal(b, nil, false); err == nil {
		t.Fatal("expected PoAD metadata without a verifier to be rejected")
	}
}

func TestValidatePoADProposalInvokesConfiguredValidator(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, PoADRoot: []byte{1}},
		block.Body{},
		block.MigrationOpt{},
	)
	b.PoADData = []byte{2}
	validator := &fakePoADBlockValidator{}

	if err := validatePoADProposal(b, validator, false); err != nil {
		t.Fatalf("expected configured validator to accept test block: %v", err)
	}

	if validator.calls != 1 {
		t.Fatalf("expected validator to be called once, got %d", validator.calls)
	}
}

func TestValidatePoADProposalPropagatesVerifierFailure(t *testing.T) {
	b := block.NewBlock(
		block.Header{Type: block.TxBlockType, PoADRoot: []byte{1}},
		block.Body{},
		block.MigrationOpt{},
	)
	b.PoADData = []byte{2}
	validator := &fakePoADBlockValidator{err: errors.New("bad signature")}

	if err := validatePoADProposal(b, validator, false); err == nil {
		t.Fatal("expected verifier error to reject proposal")
	}
}

func TestValidatePoADProposalExemptsMigrationBlockFromRequiredMetadata(t *testing.T) {
	b := block.NewBlock(block.Header{Type: block.MigrationBlockType}, block.Body{}, block.MigrationOpt{})
	if err := validatePoADProposal(b, &fakePoADBlockValidator{}, true); err != nil {
		t.Fatalf("expected migration block to be exempt from transaction PoAD policy: %v", err)
	}
}
