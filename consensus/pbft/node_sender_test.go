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
		pbftMeta: newConsensusMeta(config.ConsensusNodeCfg{NodeNum: 3, ShardNum: 3}, config.LocalParams{ShardID: 1, NodeID: 1}),
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
