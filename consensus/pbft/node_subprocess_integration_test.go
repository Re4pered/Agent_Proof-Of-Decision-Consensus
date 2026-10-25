package pbft

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/block"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network/clientconnrpc"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/nodetopo"
)

type pbftSubprocessChild struct {
	cmd    *exec.Cmd
	lines  chan string
	output *strings.Builder
}

// TestPBFTPreprepareBroadcastAcrossProcesses launches three independent copies
// of the Go test binary. Each child hosts its own production RPCConn gRPC server
// and dispatches the received Preprepare through Node.handleMessage.
func TestPBFTPreprepareBroadcastAcrossProcesses(t *testing.T) {
	if os.Getenv("POAD_PBFT_SUBPROCESS_HELPER") == "1" {
		return
	}

	const shardID int64 = 1
	const replicaCount = 3

	addresses := make([]string, replicaCount)
	for i := range addresses {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve address for replica %d: %v", i, err)
		}
		addresses[i] = listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatalf("release reserved address for replica %d: %v", i, err)
		}
	}
	addressList := strings.Join(addresses, ",")

	children := make([]pbftSubprocessChild, replicaCount)
	defer func() {
		for i := range children {
			if children[i].cmd != nil && children[i].cmd.Process != nil {
				_ = children[i].cmd.Process.Kill()
				_, _ = children[i].cmd.Process.Wait()
			}
		}
	}()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for nodeID := 0; nodeID < replicaCount; nodeID++ {
		cmd := exec.Command(executable, "-test.run=^TestPBFTSubprocessReplicaHelper$")
		cmd.Env = append(os.Environ(),
			"POAD_PBFT_SUBPROCESS_HELPER=1",
			"POAD_PBFT_NODE_ID="+strconv.Itoa(nodeID),
			"POAD_PBFT_ADDRESSES="+addressList,
		)
		stdout, pipeErr := cmd.StdoutPipe()
		if pipeErr != nil {
			t.Fatalf("create stdout pipe for replica %d: %v", nodeID, pipeErr)
		}
		var output strings.Builder
		children[nodeID] = pbftSubprocessChild{cmd: cmd, lines: make(chan string, 16), output: &output}
		if err := cmd.Start(); err != nil {
			t.Fatalf("start replica process %d: %v", nodeID, err)
		}
		go func(index int, reader *bufio.Scanner) {
			for reader.Scan() {
				line := reader.Text()
				children[index].output.WriteString(line + "\n")
				children[index].lines <- line
			}
			close(children[index].lines)
		}(nodeID, bufio.NewScanner(stdout))
	}

	// Wait for every child to report readiness and for its TCP listener to accept
	// connections. These are separate OS processes, not goroutines in this test.
	for nodeID := range children {
		waitForChildLine(t, &children[nodeID], "POAD_READY", 10*time.Second)
		waitForTCPListener(t, addresses[nodeID], 5*time.Second)
	}

	networkAddresses := make(map[nodetopo.NodeInfo]string, replicaCount)
	for nodeID, address := range addresses {
		networkAddresses[nodetopo.NodeInfo{ShardID: shardID, NodeID: int64(nodeID)}] = address
	}
	leader := senderValidationNode()
	leader.pbftMeta.lp.ShardID = shardID
	leader.pbftMeta.lp.NodeID = 0
	leader.conn = network.NewConnHandler(clientconnrpc.NewRPCConn(
		nodetopo.NodeInfo{ShardID: shardID, NodeID: 0},
		networkAddresses,
	))
	proposal := &message.Proposal{
		Block: block.NewBlock(block.Header{Number: 1}, block.Body{}, block.MigrationOpt{}),
	}
	digest, err := proposal.Hash()
	if err != nil {
		t.Fatal(err)
	}
	preprepare := &message.PreprepareMsg{
		P: *proposal, Digest: digest, ShardID: shardID, NodeID: 0, View: 0, Seq: 0,
	}
	if err := leader.broadcastMsgInnerShard(context.Background(), preprepare); err != nil {
		t.Fatalf("broadcast preprepare to child processes: %v", err)
	}

	for nodeID := range children {
		line := waitForChildLine(t, &children[nodeID], "POAD_RESULT_ACCEPTED", 10*time.Second)
		if !strings.Contains(line, "POAD_RESULT_ACCEPTED") {
			t.Errorf("replica process %d returned unexpected result %q", nodeID, line)
		}
		if err := children[nodeID].cmd.Wait(); err != nil {
			t.Errorf("replica process %d failed: %v; output: %s", nodeID, err, children[nodeID].output.String())
		}
		children[nodeID].cmd = nil
	}
}

// TestPBFTSubprocessReplicaHelper is the child-process entry point used by the
// integration test. It starts the real RPCConn listener and handles one
// proposal before exiting. It is a no-op during the ordinary parent test run.
func TestPBFTSubprocessReplicaHelper(t *testing.T) {
	if os.Getenv("POAD_PBFT_SUBPROCESS_HELPER") != "1" {
		return
	}
	nodeID, err := strconv.Atoi(os.Getenv("POAD_PBFT_NODE_ID"))
	if err != nil {
		t.Fatalf("parse child node ID: %v", err)
	}
	addresses := strings.Split(os.Getenv("POAD_PBFT_ADDRESSES"), ",")
	if len(addresses) != 3 || nodeID < 0 || nodeID >= len(addresses) {
		t.Fatalf("invalid child network configuration: node=%d addresses=%q", nodeID, addresses)
	}

	const shardID int64 = 1
	networkAddresses := make(map[nodetopo.NodeInfo]string, len(addresses))
	for id, address := range addresses {
		networkAddresses[nodetopo.NodeInfo{ShardID: shardID, NodeID: int64(id)}] = address
	}
	conn := clientconnrpc.NewRPCConn(
		nodetopo.NodeInfo{ShardID: shardID, NodeID: int64(nodeID)},
		networkAddresses,
	)
	go func() {
		if err := conn.ListenStart(); err != nil {
			fmt.Fprintf(os.Stderr, "replica %d listener failed: %v\n", nodeID, err)
		}
	}()
	fmt.Println("POAD_READY")

	node := senderValidationNode()
	node.pbftMeta.lp.ShardID = shardID
	node.pbftMeta.lp.NodeID = int64(nodeID)
	node.pbftMsgHandler = make(map[string]messageHandleFunc)
	node.iop = &acceptingProposalOp{}
	node.conn = network.NewConnHandler(conn)
	node.registerHandleFunc()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, wrapped := range conn.DrainMsgBuffer() {
			if err := node.handleMessage(context.Background(), wrapped); err != nil {
				t.Fatalf("replica %d rejected received proposal: %v", nodeID, err)
			}
			fmt.Printf("POAD_RESULT_ACCEPTED node=%d type=%s\n", nodeID, wrapped.GetMsgType())
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("replica %d timed out waiting for preprepare", nodeID)
}

func waitForChildLine(t *testing.T, process *pbftSubprocessChild, marker string, timeout time.Duration) string {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-process.lines:
			if !ok {
				t.Fatalf("child exited before emitting %s; output: %s", marker, process.output.String())
			}
			if strings.Contains(line, marker) {
				return line
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s; output: %s", marker, process.output.String())
		}
	}
}

func waitForTCPListener(t *testing.T, address string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child TCP listener %s did not become ready", address)
}
