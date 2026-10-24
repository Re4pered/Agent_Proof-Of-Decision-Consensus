package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "net/http/pprof"

	"github.com/HuangLab-SYSU/block-emulator-x/cmd/loadnetwork"
	"github.com/HuangLab-SYSU/block-emulator-x/config"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/pbft"
	"github.com/HuangLab-SYSU/block-emulator-x/consensus/poad"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/logger"
	"github.com/HuangLab-SYSU/block-emulator-x/pkg/network"
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
	poadIntentManifestPath = flag.String(
		"poad-intent-manifest",
		"",
		"path to the AgentSupervisor payment-intent JSONL manifest",
	)
	poadIntentManifestSHA256 = flag.String(
		"poad-intent-manifest-sha256",
		"",
		"trusted SHA-256 digest of the exact payment-intent manifest bytes",
	)
	poadDecisionEndpoint = flag.String(
		"poad-agent-supervisor-endpoint",
		"",
		"HTTP(S) endpoint that collects independently signed AgentSupervisor decisions",
	)
	poadDecisionTimeout = flag.Int(
		"poad-agent-supervisor-timeout-seconds",
		15,
		"AgentSupervisor decision request timeout in seconds",
	)
	poadRequiredVotes = flag.Int(
		"poad-required-votes",
		2,
		"minimum ACCEPT decisions required to finalize a PoAD payment block",
	)
)

func validatePoADStartupConfig(required bool, registryPath, registrySHA256, manifestPath, manifestSHA256 string) error {
	if required && (registryPath == "" || manifestPath == "") {
		return fmt.Errorf("-poad-required needs a trusted identity registry and a payment-intent manifest")
	}

	if manifestPath != "" && !required {
		return fmt.Errorf(
			"-poad-intent-manifest requires -poad-required so transaction blocks cannot bypass " +
				"intent verification by omitting PoAD metadata",
		)
	}

	if (registryPath == "") != (registrySHA256 == "") {
		return fmt.Errorf("-poad-registry and -poad-registry-sha256 must be provided together")
	}

	if (manifestPath == "") != (manifestSHA256 == "") {
		return fmt.Errorf("-poad-intent-manifest and -poad-intent-manifest-sha256 must be provided together")
	}

	if manifestPath != "" && registryPath == "" {
		return fmt.Errorf("PoAD intent manifest validation requires a trusted -poad-registry")
	}

	return nil
}

func validatePoADProviderConfig(required bool, endpoint string, timeoutSeconds, requiredVotes int, manifestPath string) error {
	if endpoint == "" {
		if required {
			return fmt.Errorf("-poad-required needs -poad-agent-supervisor-endpoint so the leader can build PoAD metadata")
		}

		return nil
	}

	if !required {
		return fmt.Errorf("-poad-agent-supervisor-endpoint requires -poad-required")
	}

	if manifestPath == "" {
		return fmt.Errorf("-poad-agent-supervisor-endpoint requires -poad-intent-manifest")
	}

	if timeoutSeconds <= 0 {
		return fmt.Errorf("AgentSupervisor timeout must be positive")
	}

	if requiredVotes < 1 {
		return fmt.Errorf("-poad-required-votes must be positive")
	}

	return nil
}

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

	if err := validatePoADStartupConfig(
		*poadRequired,
		*poadRegistryPath,
		*poadRegistrySHA256,
		*poadIntentManifestPath,
		*poadIntentManifestSHA256,
	); err != nil {
		log.Fatal(err)
	}

	var poadValidator *poad.BlockMetadataValidator

	var poadRegistry *poad.IdentityRegistry

	var intentLinks []poad.TraceIntentLink

	if *poadRegistryPath != "" {
		registryBytes, readErr := os.ReadFile(*poadRegistryPath)
		if readErr != nil {
			log.Fatal(fmt.Errorf("read PoAD identity registry: %w", readErr))
		}

		registry, registryErr := poad.LoadIdentityRegistryJSON(registryBytes, *poadRegistrySHA256)
		if registryErr != nil {
			log.Fatal(fmt.Errorf("load trusted PoAD identity registry: %w", registryErr))
		}

		poadRegistry = registry

		if *poadIntentManifestPath != "" {
			manifest, openErr := os.Open(*poadIntentManifestPath)
			if openErr != nil {
				log.Fatal(fmt.Errorf("open PoAD payment-intent manifest: %w", openErr))
			}

			links, loadErr := poad.LoadTraceIntentLinksVerified(manifest, *poadIntentManifestSHA256)
			closeErr := manifest.Close()

			if loadErr != nil {
				log.Fatal(fmt.Errorf("load trusted PoAD payment-intent manifest: %w", loadErr))
			}

			if closeErr != nil {
				log.Fatal(fmt.Errorf("close PoAD payment-intent manifest: %w", closeErr))
			}

			intentLinks = links
			poadValidator, err = poad.NewBlockMetadataValidatorWithIntentLinks(registry, links)
		} else {
			poadValidator, err = poad.NewBlockMetadataValidator(registry)
		}

		if err != nil {
			log.Fatal(fmt.Errorf("create PoAD block validator: %w", err))
		}
	}

	if err := validatePoADProviderConfig(
		*poadRequired,
		*poadDecisionEndpoint,
		*poadDecisionTimeout,
		*poadRequiredVotes,
		*poadIntentManifestPath,
	); err != nil {
		log.Fatal(err)
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

		log.Printf(
			"PoAD proposal validation configured: required=%t registry=%s intent_manifest=%s",
			*poadRequired, *poadRegistryPath, *poadIntentManifestPath,
		)
	} else {
		log.Printf("PoAD proposal validation is optional and no identity registry is configured; legacy blocks without PoAD remain accepted")
	}

	if *poadDecisionEndpoint != "" {
		provider, providerErr := poad.NewAgentSupervisorDecisionProvider(
			*poadDecisionEndpoint,
			time.Duration(*poadDecisionTimeout)*time.Second,
		)
		if providerErr != nil {
			log.Fatal(fmt.Errorf("configure AgentSupervisor decision provider: %w", providerErr))
		}

		if poadRegistry == nil || len(intentLinks) == 0 {
			log.Fatal("AgentSupervisor decision provider requires a trusted registry and loaded intent manifest")
		}

		builder, builderErr := poad.NewPaymentProposalBuilder(
			poadRegistry,
			intentLinks,
			provider,
			*poadRequiredVotes,
		)
		if builderErr != nil {
			log.Fatal(fmt.Errorf("create PoAD proposal builder: %w", builderErr))
		}

		if err := consensusNode.ConfigurePoADProposalBuilder(builder); err != nil {
			log.Fatal(fmt.Errorf("configure PoAD proposal builder: %w", err))
		}

		log.Printf("PoAD AgentSupervisor decision provider configured: required_votes=%d", *poadRequiredVotes)
	}

	// Wait other nodes to start listening.
	time.Sleep(nodeWaitingTime)

	consensusNode.Start()
}
