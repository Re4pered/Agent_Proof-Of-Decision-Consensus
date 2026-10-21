package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/HuangLab-SYSU/block-emulator-x/cmd/loadnetwork"
	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/pbft"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/poad"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/logger"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network"

	_ "net/http/pprof"
)

const (
	nodeWaitingTime     = 5 * time.Second
	pprofPortLowerBound = 5000
)

var (
	configPath = flag.String("config", "config.yaml", "path to config file")
	// pprof is a tool to sample the program and collect the runtime data.
	pprofPort = flag.Int(
		"pprof-port",
		0,
		fmt.Sprintf("port to serve pprof; the port should be larger than %d", pprofPortLowerBound),
	)
	poadRequired = flag.Bool(
		"poad-required",
		false,
		"reject transaction blocks without valid PoAD metadata",
	)
	poadRegistryPath = flag.String(
		"poad-registry",
		"",
		"path to the trusted PoAD identity registry JSON snapshot",
	)
	poadRegistrySHA256 = flag.String(
		"poad-registry-sha256",
		"",
		"trusted SHA-256 digest of the exact PoAD registry file bytes",
	)
)

func main() {
	flag.Parse()

	lp, err := config.LoadLocalParams()
	if err != nil {
		log.Fatal(fmt.Errorf("load local parameters failed: %w", err))
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(fmt.Errorf("load config: %w", err))
	}

	if *poadRequired && *poadRegistryPath == "" {
		log.Fatal("-poad-required needs both -poad-registry and -poad-registry-sha256")
	}

	if (*poadRegistryPath == "") != (*poadRegistrySHA256 == "") {
		log.Fatal("-poad-registry and -poad-registry-sha256 must be provided together")
	}

	var poadValidator *poad.BlockMetadataValidator

	if *poadRegistryPath != "" {
		registryBytes, readErr := os.ReadFile(*poadRegistryPath)
		if readErr != nil {
			log.Fatal(fmt.Errorf("read PoAD identity registry: %w", readErr))
		}

		registry, registryErr := poad.LoadIdentityRegistryJSON(registryBytes, *poadRegistrySHA256)

		if registryErr != nil {
			log.Fatal(fmt.Errorf("load trusted PoAD identity registry: %w", registryErr))
		}

		poadValidator, err = poad.NewBlockMetadataValidator(registry)
		if err != nil {
			log.Fatal(fmt.Errorf("create PoAD block validator: %w", err))
		}
	}

	if pprofPort != nil && *pprofPort >= pprofPortLowerBound {
		go func() { log.Println(http.ListenAndServe(fmt.Sprintf(":%d", *pprofPort), nil)) }()
	}

	// set the default logger here
	if err = logger.InitLogger(lp, cfg.LogCfg); err != nil {
		log.Fatal(fmt.Errorf("init logger failed: %w", err))
	}

	defer logger.CloseLoggerFile()

	p2p, nodeM, err := loadnetwork.PrepareNetworkByCfg(cfg, lp)
	if err != nil {
		log.Fatal(fmt.Errorf("prepare network: %w", err))
	}

	consensusNode, err := pbft.NewPBFTNode(network.NewConnHandler(p2p), nodeM, cfg.ConsensusNodeCfg, *lp)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to create a PBFT node: %w", err))
	}

	if poadValidator != nil {
		if err := consensusNode.ConfigurePoADValidation(poadValidator, *poadRequired); err != nil {
			log.Fatal(fmt.Errorf("configure PoAD proposal validation: %w", err))
		}

		log.Printf("PoAD proposal validation configured: required=%t registry=%s", *poadRequired, *poadRegistryPath)
	} else {
		log.Printf("PoAD proposal validation is optional and no identity registry is configured; legacy blocks without PoAD remain accepted")
	}

	// Wait other nodes to start listening.
	time.Sleep(nodeWaitingTime)

	consensusNode.Start()
}
