package pbft

import (
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/nodetopo"
)

func senderValidationNode() *Node {
	resolver := nodetopo.NewTopoGetter(
		map[int64]nodetopo.NodeInfo{1: {ShardID: 1, NodeID: 0}},
		map[int64][]nodetopo.NodeInfo{
			1: {
				{ShardID: 1, NodeID: 0},
				{ShardID: 1, NodeID: 1},
				{ShardID: 1, NodeID: 2},
			},
			2: {{ShardID: 2, NodeID: 0}},
		},
	)
	return &Node{
		resolver: resolver,
		pbftMeta: newConsensusMeta(config.ConsensusNodeCfg{BlockchainCfg: config.BlockchainCfg{SystemCfg: config.SystemCfg{NodeNum: 3, ShardNum: 3}}}, config.LocalParams{ShardID: 1, NodeID: 1}),
	}
}

func TestValidateConsensusSenderAcceptsConfiguredShardMember(t *testing.T) {
	node := senderValidationNode()
	if err := node.validateConsensusSender(1, 2); err != nil {
		t.Fatalf("expected configured shard member to pass: %v", err)
	}
}

func TestValidateConsensusSenderRejectsUnknownNode(t *testing.T) {
	node := senderValidationNode()
	if err := node.validateConsensusSender(1, 99); err == nil {
		t.Fatal("expected unknown node ID to be rejected")
	}
}

func TestValidateConsensusSenderRejectsCrossShardMessage(t *testing.T) {
	node := senderValidationNode()
	if err := node.validateConsensusSender(2, 0); err == nil {
		t.Fatal("expected cross-shard consensus message to be rejected")
	}
}

func TestValidateCatchupRequestAcceptsConfiguredShardMember(t *testing.T) {
	node := senderValidationNode()
	if err := node.validateCatchupRequest(1, 2, 1); err != nil {
		t.Fatalf("expected valid catch-up request to pass: %v", err)
	}
}

func TestValidateCatchupRequestRejectsCrossShardRequester(t *testing.T) {
	node := senderValidationNode()
	if err := node.validateCatchupRequest(2, 0, 1); err == nil {
		t.Fatal("expected cross-shard catch-up request to be rejected")
	}
}

func TestValidateCatchupRequestRejectsNegativeStartHeight(t *testing.T) {
	node := senderValidationNode()
	if err := node.validateCatchupRequest(1, 1, -1); err == nil {
		t.Fatal("expected negative catch-up start height to be rejected")
	}
}
