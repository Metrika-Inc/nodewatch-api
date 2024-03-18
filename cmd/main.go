// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"eth2-crawler/crawler"
	"eth2-crawler/output"
	"eth2-crawler/resolver/ipmapper"
	peerStore "eth2-crawler/store/peerstore/mongo"
	recordStore "eth2-crawler/store/record/mongo"
	httpTransport "eth2-crawler/transport/http"
	"eth2-crawler/utils/config"
)

func main() {

	ctx, cancel := context.WithCancel(context.Background())

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	var wg sync.WaitGroup
	cfgPath := flag.String("p", "./cmd/config/config.dev.yaml", "The configuration path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("error loading configuration: %s", err.Error())
	}

	peerStore, err := peerStore.New(cfg.Database)
	if err != nil {
		log.Fatalf("error Initializing the peer store: %s", err.Error())
	}

	historyStore, err := recordStore.New(cfg.Database)
	if err != nil {
		log.Fatalf("error Initializing the record store: %s", err.Error())
	}

	// resolverService, err := ipdata.New(cfg.Resolver.APIKey, time.Duration(cfg.Resolver.Timeout)*time.Second)
	resolverService := ipmapper.New()

	// if err != nil {
	// 	log.Fatalf("error Initializing the ip resolver: %s", err.Error())
	// }

	fOutputChan := make(chan interface{}, cfg.Crawler.Concurrency)

	var kafkaConfig *output.KafkaConfig
	if cfg.Kafka != nil {
		kafkaConfig = &output.KafkaConfig{
			BootstrapServers: cfg.Kafka.BootstrapServers,
			Topic:            cfg.Kafka.Topic,
			Timeout:          cfg.Kafka.Timeout,
		}
	}

	fOutput, err := output.New(cfg.FileOutput.Path, kafkaConfig, fOutputChan)
	if err != nil {
		log.Fatalf("error Initializing the file output: %s", err.Error())
	}

	// Start the http server
	httpHdlr := httpTransport.NewHttpTransport(cfg.Server)
	httpHdlr.RegisterRoutes()
	go httpHdlr.Start(ctx, &wg)

	go crawler.Start(ctx, &wg, cfg, peerStore, historyStore, resolverService, fOutput)

	// Block until terminate called
	<-exit
	fmt.Println("=====================================")
	fmt.Println("Got termination signal, cancelling context abd shutting down")
	fmt.Println("=====================================")

	// Cancelled the context
	cancel()

	// Wait on wg to finish
	wg.Wait()
}

func startHttpServer(ctx context.Context, cfg *config.Server) {
	// Start the http server

}
