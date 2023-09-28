// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

// Package config holds the config file parsing functionality
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/configs"
	"github.com/protolambda/ztyp/tree"
	"gopkg.in/yaml.v2"
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
}

type Crawler struct {
	Concurrency           int                 `yaml:"concurrency,omitempty"`
	ConnectionRetries     int                 `yaml:"connection_retries,omitempty"`
	UpdateFreqMin         int                 `yaml:"update_freq_min,omitempty"`
	GenesisValidatorsRoot string              `yaml:"genesis_validators_root,omitempty"`
	ForkDigestStr         string              `yaml:"fork_digest,omitempty"`
	ForkDigest            *common.ForkDigest  `yaml:"-"`
	ForkDecoder           *beacon.ForkDecoder `yaml:"-"`
}

type FileOutput struct {
	Path string `yaml:"path,omitempty"`
}

// Server holds data necessary for server configuration
type Server struct {
	Port              string   `yaml:"port,omitempty"`
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

	// Validate the fork digest is valid
	root, err := hex.DecodeString(cfg.Crawler.GenesisValidatorsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to decode genesis validator root, err: %s", err)
	}

	var treeRoot tree.Root
	copy(treeRoot[:], root)
	cfg.Crawler.ForkDecoder = beacon.NewForkDecoder(configs.Mainnet, treeRoot)

	cfgDigest := new(common.ForkDigest)
	cfgDigest.UnmarshalText([]byte(cfg.Crawler.ForkDigestStr))

	switch *cfgDigest {
	case cfg.Crawler.ForkDecoder.Genesis:
		fallthrough
	case cfg.Crawler.ForkDecoder.Altair:
		fallthrough
	case cfg.Crawler.ForkDecoder.Bellatrix:
		fallthrough
	case cfg.Crawler.ForkDecoder.Capella:
		fallthrough
	case cfg.Crawler.ForkDecoder.Deneb:
		cfg.Crawler.ForkDigest = cfgDigest
	default:
		// Unknown fork digest
		return nil, fmt.Errorf("unknown fork digest: %s", cfg.Crawler.ForkDigestStr)
	}

	return cfg, nil
}
