// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

// Package config holds the config file parsing functionality
package config

import (
	"encoding/hex"
	"errors"
	"eth2-crawler/cmd/network"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/configs"
	"github.com/protolambda/ztyp/tree"
	"gopkg.in/yaml.v2"
)

const (
	MAINNET_ALTAIR_FORK_EPOCH    = 74240
	MAINNET_BELLATRIX_FORK_EPOCH = 144896
	MAINNET_CAPELLA_FORK_EPOCH   = 194048
	MAINNET_DENEB_FORK_EPOCH     = 269568
	MAINNET_PECTRA_FORK_EPOCH    = 999999999
)

const KafkaDefaultTimeout = 10 * time.Second

// Configuration holds data necessary for configuring application
type Configuration struct {
	Server     *Server     `yaml:"server,omitempty"`
	Database   *Database   `yaml:"database,omitempty"`
	Resolver   *Resolver   `yaml:"resolver,omitempty"`
	FileOutput *FileOutput `yaml:"fileOutput,omitempty"`
	Crawler    *Crawler    `yaml:"crawler,omitempty"`
	Kafka      *Kafka      `yaml:"kafka,omitempty"`
	Fork       *Fork       `yaml:"fork,omitempty"`
	Network    *Network    `yaml:"network,omitempty"`
}

type Network struct {
	Name      string   `yaml:"name,omitempty"`
	Bootnodes []string `yaml:"bootnodes,omitempty"`
}

type Crawler struct {
	Concurrency           int    `yaml:"concurrency,omitempty"`
	ConnectionRetries     int    `yaml:"connection_retries,omitempty"`
	UpdateFreqMin         int    `yaml:"update_freq_min,omitempty"`
	GenesisValidatorsRoot string `yaml:"genesis_validators_root,omitempty"`
	GenesisTime           int64  `yaml:"genesis_time,omitempty"`
	SecondsPerSlot        int64  `yaml:"seconds_per_slot,omitempty"`
	SlotsPerEpoch         int64  `yaml:"slots_per_epoch,omitempty"`

	ForkDecoder *beacon.ForkDecoder `yaml:"-"`
}

type FileOutput struct {
	Path string `yaml:"path,omitempty"`
}

// Server holds data necessary for server configuration
type Server struct {
	Port              int      `yaml:"port,omitempty"`
	ReadTimeout       int      `yaml:"read_timeout_seconds,omitempty"`
	ReadHeaderTimeout int      `yaml:"read_header_timeout_seconds,omitempty"`
	WriteTimeout      int      `yaml:"write_timeout_seconds,omitempty"`
	CORS              []string `yaml:"cors,omitempty"`
}

// Database is a MongoDB config
type Database struct {
	URI               string `yaml:"-"`
	Timeout           int    `yaml:"request_timeout_sec"`
	Database          string `yaml:"database"`
	Collection        string `yaml:"collection"`
	HistoryCollection string `yaml:"history_collection"`
}

// Resolver provides config for resolver
type Resolver struct {
	APIKey  string `yaml:"-"`
	Timeout int    `yaml:"request_timeout_sec"`
}

type Kafka struct {
	Topic               string        `yaml:"topic,omitempty"`
	BootstrapServersStr string        `yaml:"bootstrap_servers,omitempty"`
	BootstrapServers    []string      `yaml:"-"`
	Timeout             time.Duration `yaml:"timeout,omitempty"`
}

type ForkConfig struct {
	Supported  bool   `yaml:"supported,omitempty"`
	ForkDigest string `yaml:"fork_digest,omitempty"`
	ForkEpoch  int64  `yaml:"fork_epoch,omitempty"`
}
type Fork struct {
	Genesis   ForkConfig `yaml:"genesis,omitempty"`
	Altair    ForkConfig `yaml:"altair,omitempty"`
	Bellatrix ForkConfig `yaml:"bell,omitempty"`
	Capella   ForkConfig `yaml:"capella,omitempty"`
	Deneb     ForkConfig `yaml:"deneb,omitempty"`
	Electra   ForkConfig `yaml:"electra,omitempty"`
}

func loadDatabaseURI() (string, error) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return "", errors.New("MONGODB_URI is required")
	}
	return mongoURI, nil
}

func loadResolverAPIKey() (string, error) {
	resolverAPIKey := os.Getenv("RESOLVER_API_KEY")
	if resolverAPIKey == "" {
		return "", errors.New("RESOLVER_API_KEY is required")
	}
	return resolverAPIKey, nil
}

// Load returns Configuration struct
func Load(path string) (*Configuration, error) {
	bytes, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("error reading config file, %w", err)
	}

	var cfg = new(Configuration)

	if err = yaml.Unmarshal(bytes, cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct, %w", err)
	}

	if cfg.Kafka != nil {
		cfg.Kafka.BootstrapServers = strings.Split(cfg.Kafka.BootstrapServersStr, ",")
		if cfg.Kafka.Timeout == 0 {
			cfg.Kafka.Timeout = KafkaDefaultTimeout
		}
	}

	// load envs
	cfg.Database.URI, err = loadDatabaseURI()
	if err != nil {
		return nil, err
	}

	err = loadForkDefaults(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadForkDefaults(cfg *Configuration) error {

	root, err := hex.DecodeString(cfg.Crawler.GenesisValidatorsRoot)
	if err != nil {
		return fmt.Errorf("failed to decode genesis validator root, err: %s", err)
	}

	var treeRoot tree.Root
	copy(treeRoot[:], root)

	switch cfg.Network.Name {
	case "mainnet":
		cfg.Crawler.ForkDecoder = beacon.NewForkDecoder(configs.Mainnet, treeRoot)

		// config doesn't contain the fork epochs, add them manually
		configs.Mainnet.ALTAIR_FORK_EPOCH = MAINNET_ALTAIR_FORK_EPOCH
		configs.Mainnet.BELLATRIX_FORK_EPOCH = MAINNET_BELLATRIX_FORK_EPOCH
		configs.Mainnet.CAPELLA_FORK_EPOCH = MAINNET_CAPELLA_FORK_EPOCH
		configs.Mainnet.DENEB_FORK_EPOCH = MAINNET_DENEB_FORK_EPOCH
		configs.Mainnet.ELECTRA_FORK_EPOCH = MAINNET_PECTRA_FORK_EPOCH
		log.Info("Using mainnet fork decoder")
	case "goerli":
		cfg.Crawler.ForkDecoder = beacon.NewForkDecoder(network.Goerli, treeRoot)
		log.Info("Using goerli fork decoder")
	case "holesky":
		cfg.Crawler.ForkDecoder = beacon.NewForkDecoder(network.Holesky, treeRoot)
		log.Info("Using holesky fork decoder")
	default:
		log.Info("no network specified, using configured epochs and fork digests")
	}

	// Populate the fork digest if not set but supported

	if cfg.Fork.Altair.ForkDigest == "" && cfg.Fork.Altair.Supported {
		cfg.Fork.Altair.ForkDigest = cfg.Crawler.ForkDecoder.Altair.String()
		cfg.Fork.Altair.ForkEpoch = int64(cfg.Crawler.ForkDecoder.Spec.ALTAIR_FORK_EPOCH)
	}
	if cfg.Fork.Bellatrix.ForkDigest == "" {
		cfg.Fork.Bellatrix.ForkDigest = cfg.Crawler.ForkDecoder.Bellatrix.String()
		cfg.Fork.Bellatrix.ForkEpoch = int64(cfg.Crawler.ForkDecoder.Spec.BELLATRIX_FORK_EPOCH)
	}
	if cfg.Fork.Capella.ForkDigest == "" {
		cfg.Fork.Capella.ForkDigest = cfg.Crawler.ForkDecoder.Capella.String()
		cfg.Fork.Capella.ForkEpoch = int64(cfg.Crawler.ForkDecoder.Spec.CAPELLA_FORK_EPOCH)
	}
	if cfg.Fork.Deneb.ForkDigest == "" {
		cfg.Fork.Deneb.ForkDigest = cfg.Crawler.ForkDecoder.Deneb.String()
		cfg.Fork.Deneb.ForkEpoch = int64(cfg.Crawler.ForkDecoder.Spec.DENEB_FORK_EPOCH)
	}
	if cfg.Fork.Electra.ForkDigest == "" {
		cfg.Fork.Electra.ForkDigest = cfg.Crawler.ForkDecoder.Electra.String()
		cfg.Fork.Electra.ForkEpoch = int64(cfg.Crawler.ForkDecoder.Spec.ELECTRA_FORK_EPOCH)
	}

	return nil
}
