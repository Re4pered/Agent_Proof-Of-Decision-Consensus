package committee

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log/slog"

	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/core/transaction"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/message"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network/rpcserver"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/nodetopo"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/partition"
	"github.com/HuangLab-SYSU/block-emulator-x/supervisor/txsource"
)

type StaticRelayCommittee struct {
	r    nodetopo.NodeMapper  // r give the information of other nodes.
	conn *network.ConnHandler // conn is the p2p-connections among consensus nodes, i.e., network layer.

	txSource    txsource.TxSource // txSource brings the txs into the blockchain system.
	sl          stopLogic         // sl is the logic of stop.
	unsentTxNum int64

	// The opt-in smoke mode observes unique hashes of injected and committed
	// intra-shard transactions. It does not count empty blocks or relay stages.
	smokeInjected  map[string]struct{}
	smokeCommitted map[string]struct{}
	cfg            config.SupervisorCfg
}

func NewStaticRelayCommittee(
	conn *network.ConnHandler,
	r nodetopo.NodeMapper,
	cfg config.SupervisorCfg,
) (*StaticRelayCommittee, error) {
	ts, err := txsource.NewTxSource(cfg.TxSourceCfg)
	if err != nil {
		return nil, fmt.Errorf("NewTxSource failed: %w", err)
	}

	return &StaticRelayCommittee{
		r:              r,
		conn:           conn,
		txSource:       ts,
		sl:             stopLogic{stopThreshold: cfg.ShardNum * stopThresholdPerShard, stopCnt: 0},
		unsentTxNum:    cfg.TxNumber,
		smokeInjected:  make(map[string]struct{}),
		smokeCommitted: make(map[string]struct{}),
		cfg:            cfg,
	}, nil
}

func (s *StaticRelayCommittee) SendTxsAndConsensus(ctx context.Context) error {
	if err := s.readTxsAndSend(ctx); err != nil {
		return fmt.Errorf("readTxsAndSend failed: %w", err)
	}

	return nil
}

func (s *StaticRelayCommittee) HandleMsg(_ context.Context, msg *rpcserver.WrappedMsg) error {
	if msg.GetMsgType() != message.RelayBlockInfoMessageType {
		slog.Info("unknown expected msg type", "type", msg.GetMsgType())
		return nil
	}

	var bInfo message.RelayBlockInfoMsg
	if err := gob.NewDecoder(bytes.NewReader(msg.GetPayload())).Decode(&bInfo); err != nil {
		return fmt.Errorf("decode relayBlockInfoMsg: %w", err)
	}

	if s.cfg.StopOnAllCommitted {
		// The smoke mode is intentionally limited to local payments. Only count
		// transactions actually injected by this supervisor and observed in a
		// committed block; ignore duplicate or unsolicited block notifications.
		if len(bInfo.Relay1Txs) > 0 || len(bInfo.Relay2Txs) > 0 {
			return fmt.Errorf("pay-only smoke received relay transactions")
		}

		for i := range bInfo.InnerShardTxs {
			hash, hashErr := bInfo.InnerShardTxs[i].Hash()
			if hashErr != nil {
				return fmt.Errorf("hash committed smoke payment: %w", hashErr)
			}

			key := string(hash)
			if _, injected := s.smokeInjected[key]; injected {
				s.smokeCommitted[key] = struct{}{}
			}
		}

		return nil
	}

	if s.unsentTxNum <= 0 &&
		len(bInfo.InnerShardTxs)+len(bInfo.Relay1Txs)+len(bInfo.Relay2Txs) == 0 {
		s.sl.stopCnt++
	} else if s.unsentTxNum <= 0 {
		s.sl.stopCnt = 0 // reset 0 if there are transactions in a block
	}

	return nil
}

func (s *StaticRelayCommittee) ShouldStop() bool {
	if s.cfg.StopOnAllCommitted {
		return s.unsentTxNum <= 0 &&
			int64(len(s.smokeInjected)) == s.cfg.TxNumber &&
			len(s.smokeCommitted) == len(s.smokeInjected)
	}

	return s.sl.stopCnt >= s.sl.stopThreshold
}

func (s *StaticRelayCommittee) readTxsAndSend(ctx context.Context) error {
	txs, err := s.txSource.ReadTxs(min(s.cfg.TxInjectionSpeed, s.unsentTxNum))
	if err != nil {
		return fmt.Errorf("failed to read txs: %w", err)
	}

	// send transactions
	shardTxs := packShardTxs(txs, s.cfg.ShardNum, s.getTxLoc)
	if err = message.SendWrappedTxs2Shards(ctx, shardTxs, s.conn, s.r); err != nil {
		return fmt.Errorf("failed to send txs to shards: %w", err)
	}

	if s.cfg.StopOnAllCommitted {
		for i := range txs {
			hash, hashErr := txs[i].Hash()
			if hashErr != nil {
				return fmt.Errorf("hash injected smoke payment: %w", hashErr)
			}

			s.smokeInjected[string(hash)] = struct{}{}
		}
	}

	s.unsentTxNum -= int64(len(txs))

	return nil
}

func (s *StaticRelayCommittee) getTxLoc(tx transaction.Transaction) int64 {
	return partition.DefaultAccountLoc(tx.Sender, s.cfg.ShardNum)
}
