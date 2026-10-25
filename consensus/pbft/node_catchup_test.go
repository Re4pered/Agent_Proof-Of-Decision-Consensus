package pbft

import (
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
)

func catchupFixture(t *testing.T) (block.Header, []message.Proposal) {
	t.Helper()
	genesis := block.Header{Number: 0}
	genesisHash, err := genesis.Hash()
	if err != nil {
		t.Fatalf("hash genesis header: %v", err)
	}

	first := block.NewBlock(block.Header{
		Number:          1,
		ParentBlockHash: genesisHash,
	}, block.Body{}, block.MigrationOpt{})
	firstHash, err := first.Header.Hash()
	if err != nil {
		t.Fatalf("hash first header: %v", err)
	}

	second := block.NewBlock(block.Header{
		Number:          2,
		ParentBlockHash: firstHash,
	}, block.Body{}, block.MigrationOpt{})
	return genesis, []message.Proposal{{Block: first}, {Block: second}}
}

func TestValidateCatchupChainStructureAcceptsContiguousBatch(t *testing.T) {
	parent, proposals := catchupFixture(t)
	if err := validateCatchupChainStructure(proposals, parent); err != nil {
		t.Fatalf("expected contiguous catch-up batch to pass, got %v", err)
	}
}

func TestValidateCatchupChainStructureRejectsHeightGap(t *testing.T) {
	parent, proposals := catchupFixture(t)
	proposals[1].Block.Number = 3
	if err := validateCatchupChainStructure(proposals, parent); err == nil {
		t.Fatal("expected catch-up batch with a height gap to be rejected")
	}
}

func TestValidateCatchupChainStructureRejectsParentHashMismatch(t *testing.T) {
	parent, proposals := catchupFixture(t)
	proposals[1].Block.ParentBlockHash = []byte("wrong-parent")
	if err := validateCatchupChainStructure(proposals, parent); err == nil {
		t.Fatal("expected catch-up batch with a parent hash mismatch to be rejected")
	}
}

func TestValidateCatchupChainStructureRejectsNilBlock(t *testing.T) {
	parent, proposals := catchupFixture(t)
	proposals[1].Block = nil
	if err := validateCatchupChainStructure(proposals, parent); err == nil {
		t.Fatal("expected catch-up batch with a nil block to be rejected")
	}
}


func TestApplyCatchupProposalsRejectsInvalidPoADBeforePersistingThatBlock(t *testing.T) {
	_, proposals := catchupFixture(t)
	wantErr := errors.New("invalid PoAD decision proof")
	validated := make([]int64, 0, len(proposals))
	persisted := make([]int64, 0, len(proposals))

	err := applyCatchupProposals(
		context.Background(),
		proposals,
		func(_ context.Context, proposal *message.Proposal) error {
			validated = append(validated, proposal.Block.Number)
			if proposal.Block.Number == 2 {
				return wantErr
			}
			return nil
		},
		func(_ context.Context, proposal *message.Proposal) error {
			persisted = append(persisted, proposal.Block.Number)
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("applyCatchupProposals() error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(validated, []int64{1, 2}) {
		t.Fatalf("validated heights = %v, want [1 2]", validated)
	}
	if !reflect.DeepEqual(persisted, []int64{1}) {
		t.Fatalf("persisted heights = %v, want only [1]", persisted)
	}
}

func TestApplyCatchupProposalsStopsAfterPersistenceFailure(t *testing.T) {
	_, proposals := catchupFixture(t)
	wantErr := errors.New("storage unavailable")
	validated := make([]int64, 0, len(proposals))
	persisted := make([]int64, 0, len(proposals))

	err := applyCatchupProposals(
		context.Background(),
		proposals,
		func(_ context.Context, proposal *message.Proposal) error {
			validated = append(validated, proposal.Block.Number)
			return nil
		},
		func(_ context.Context, proposal *message.Proposal) error {
			persisted = append(persisted, proposal.Block.Number)
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("applyCatchupProposals() error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(validated, []int64{1}) {
		t.Fatalf("validated heights = %v, want only [1]", validated)
	}
	if !reflect.DeepEqual(persisted, []int64{1}) {
		t.Fatalf("persisted heights = %v, want only [1]", persisted)
	}
}
