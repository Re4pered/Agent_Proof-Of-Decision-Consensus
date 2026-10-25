package pbft

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/consensus/pbft/basicstructs"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network/clientconnrpc"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network/rpcserver"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/nodetopo"
	"google.golang.org/grpc"
)

// TestPBFTPreprepareBroadcastOverGRPC sends a proposal through the production
// ConnHandler and RPCConn transport. Each replica has its own TCP listener and
// message buffer; no in-memory transport is used in this test.
func TestPBFTPreprepareBroadcastOverGRPC(t *testing.T) {
	for _, tc := range []struct {
		name       string
		validation error
		wantAccept bool
	}{
		{name: "valid proposal reaches all replicas", wantAccept: true},
		{name: "invalid PoAD is rejected by all replicas", validation: errors.New("invalid PoAD evidence")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const shardID int64 = 1
			const replicaCount int64 = 3

			listeners := make([]net.Listener, replicaCount)
			servers := make([]*grpc.Server, replicaCount)
			addresses := make(map[nodetopo.NodeInfo]string, replicaCount)
			for nodeID := int64(0); nodeID < replicaCount; nodeID++ {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("listen for replica %d: %v", nodeID, err)
				}
				listeners[nodeID] = listener
				addresses[nodetopo.NodeInfo{ShardID: shardID, NodeID: nodeID}] = listener.Addr().String()
			}
			t.Cleanup(func() {
				for _, server := range servers {
					if server != nil {
						server.Stop()
					}
				}
				for _, listener := range listeners {
					if listener != nil {
						_ = listener.Close()
					}
				}
			})

			connections := make([]*clientconnrpc.RPCConn, replicaCount)
			nodes := make([]*Node, replicaCount)
			ops := make([]*replicaProposalOp, replicaCount)
			for nodeID := int64(0); nodeID < replicaCount; nodeID++ {
				info := nodetopo.NodeInfo{ShardID: shardID, NodeID: nodeID}
				conn := clientconnrpc.NewRPCConn(info, addresses)
				connections[nodeID] = conn

				server := grpc.NewServer()
				servers[nodeID] = server
				rpcserver.RegisterReplicaConnServer(server, conn)
				go func(i int64) {
					if err := server.Serve(listeners[i]); err != nil {
						t.Errorf("replica %d gRPC server stopped unexpectedly: %v", i, err)
					}
				}(nodeID)

				node := senderValidationNode()
				node.pbftMeta.lp.NodeID = nodeID
				node.pbftMsgHandler = make(map[string]messageHandleFunc)
				ops[nodeID] = &replicaProposalOp{validationErr: tc.validation}
				node.iop = ops[nodeID]
				node.conn = network.NewConnHandler(conn)
				node.registerHandleFunc()
				nodes[nodeID] = node
			}

			// Wait until every real TCP listener is accepting connections before
			// broadcasting; otherwise RPCConn's asynchronous error logging could
			// hide an early connection failure from this test.
			for nodeID, listener := range listeners {
				deadline := time.Now().Add(2 * time.Second)
				for {
					conn, err := net.DialTimeout("tcp", listener.Addr().String(), 25*time.Millisecond)
					if err == nil {
						_ = conn.Close()
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("replica %d gRPC listener did not become ready: %v", nodeID, err)
					}
					time.Sleep(10 * time.Millisecond)
				}
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
				ShardID: shardID,
				NodeID:  0,
				View:    0,
				Seq:     0,
			}
			if err := nodes[0].broadcastMsgInnerShard(context.Background(), preprepare); err != nil {
				t.Fatalf("leader broadcast failed: %v", err)
			}

			for nodeID, node := range nodes {
				var inbound []*rpcserver.WrappedMsg
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					inbound = node.conn.DrainMsgBuffer()
					if len(inbound) > 0 {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if len(inbound) != 1 {
					t.Fatalf("replica %d received %d preprepare messages over gRPC, want 1", nodeID, len(inbound))
				}
				err := node.handleMessage(context.Background(), inbound[0])
				if tc.wantAccept && err != nil {
					t.Errorf("replica %d rejected valid proposal: %v", nodeID, err)
				}
				if !tc.wantAccept && err == nil {
					t.Errorf("replica %d accepted invalid PoAD proposal", nodeID)
				}
				pooled := node.pbftMeta.msgPool.ReadPreprepareMsg(basicstructs.ViewSeq{})
				if tc.wantAccept && len(pooled) != 1 {
					t.Errorf("replica %d pooled %d proposals, want 1", nodeID, len(pooled))
				}
				if !tc.wantAccept && len(pooled) != 0 {
					t.Errorf("replica %d pooled invalid proposal", nodeID)
				}
				if ops[nodeID].validateCalls != 1 {
					t.Errorf("replica %d validation calls = %d, want 1", nodeID, ops[nodeID].validateCalls)
				}
			}

		})
	}
}
