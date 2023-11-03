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
	fOutput, err := output.New(cfg.FileOutput.Path, &output.KafkaConfig{
		BootstrapServers: cfg.Kafka.BootstrapServers,
		Topic:            cfg.Kafka.Topic,
		Timeout:          cfg.Kafka.Timeout}, fOutputChan)
	if err != nil {
		log.Fatalf("error Initializing the file output: %s", err.Error())
	}

	go crawler.Start(ctx, &wg, cfg, peerStore, historyStore, resolverService, fOutput)

	// srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: graph.NewResolver(peerStore, historyStore)}))

	// router := http.NewServeMux()
	// // TODO: make playground accessible only in Dev mode
	// router.Handle("/", playground.Handler("GraphQL playground", "/query"))
	// router.Handle("/query", srv)
	// // TODO: setup proper status handler
	// router.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
	// 	fmt.Fprintf(w, "{ \"status\": \"up\" }")
	// })

	// server.Start(ctx, cfg.Server, router)

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
