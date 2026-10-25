package pbft

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/HuangLab-SYSU/block-emulator-x/consensus/pbft/basicstructs"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network/rpcserver"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/nodetopo"
)

// inMemoryPBFTNetwork exercises ConnHandler's real broadcast and message-buffer
// APIs while keeping this integration test deterministic and independent of ports.
type inMemoryPBFTNetwork struct {
	mu     sync.Mutex
	queues map[nodetopo.NodeInfo][]*rpcserver.WrappedMsg
}

type inMemoryPBFTConn struct {
	network *inMemoryPBFTNetwork
	self    nodetopo.NodeInfo
}

func (c *inMemoryPBFTConn) ListenStart() error { return nil }

func (c *inMemoryPBFTConn) DrainMsgBuffer() []*rpcserver.WrappedMsg {
	c.network.mu.Lock()
	defer c.network.mu.Unlock()

	messages := c.network.queues[c.self]
	c.network.queues[c.self] = nil
	return messages
}

func (c *inMemoryPBFTConn) SendMsg2Dest(_ context.Context, dest nodetopo.NodeInfo, msg *rpcserver.WrappedMsg) {
	c.network.mu.Lock()
	defer c.network.mu.Unlock()

	c.network.queues[dest] = append(c.network.queues[dest], msg)
}

func (c *inMemoryPBFTConn) Close() {}

func TestPBFTPreprepareBroadcastAcrossIndependentNodes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		validation error
		wantAccept bool
	}{
		{name: "valid proposal reaches all replicas", wantAccept: true},
		{name: "invalid PoAD is rejected by all replicas", validation: errors.New("invalid PoAD evidence")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &inMemoryPBFTNetwork{
				queues: make(map[nodetopo.NodeInfo][]*rpcserver.WrappedMsg),
			}
			nodes := make([]*Node, 3)
			ops := make([]*replicaProposalOp, 3)

			for nodeID := int64(0); nodeID < 3; nodeID++ {
				node := senderValidationNode()
				node.pbftMeta.lp.NodeID = nodeID
				ops[nodeID] = &replicaProposalOp{validationErr: tc.validation}
				node.iop = ops[nodeID]
				node.conn = network.NewConnHandler(&inMemoryPBFTConn{
					network: transport,
					self:    nodetopo.NodeInfo{ShardID: 1, NodeID: nodeID},
				})
				node.registerHandleFunc()
				nodes[nodeID] = node
			}

			proposal := &message.Proposal{
				Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
			}
			digest, err := proposal.Hash()
			if err != nil {
				t.Fatal(err)
			}
			preprepare := &message.PreprepareMsg{
				P:       *proposal,
				Digest:  digest,
				ShardID: 1,
				NodeID:  0,
				View:    0,
				Seq:     0,
			}

			// Use the production ConnHandler broadcast path, then drain each
			// independent node's own inbound queue and dispatch via handleMessage.
			if err := nodes[0].broadcastMsgInnerShard(context.Background(), preprepare); err != nil {
				t.Fatalf("leader broadcast failed: %v", err)
			}

			for nodeID, node := range nodes {
				inbound := node.conn.DrainMsgBuffer()
				if len(inbound) != 1 {
					t.Fatalf("node %d received %d preprepare messages, want 1", nodeID, len(inbound))
				}
				err := node.handleMessage(context.Background(), inbound[0])
				if tc.wantAccept && err != nil {
					t.Errorf("node %d rejected valid proposal: %v", nodeID, err)
				}
				if !tc.wantAccept && err == nil {
					t.Errorf("node %d accepted invalid PoAD proposal", nodeID)
				}

				pooled := node.pbftMeta.msgPool.ReadPreprepareMsg(basicstructs.ViewSeq{})
				if tc.wantAccept && len(pooled) != 1 {
					t.Errorf("node %d pooled %d proposals, want 1", nodeID, len(pooled))
				}
				if !tc.wantAccept && len(pooled) != 0 {
					t.Errorf("node %d pooled invalid proposal", nodeID)
				}
				if ops[nodeID].validateCalls != 1 {
					t.Errorf("node %d validation calls = %d, want 1", nodeID, ops[nodeID].validateCalls)
				}
			}
		})
	}
}
