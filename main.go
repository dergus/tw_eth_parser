package main

import (
	"context"
	"log"
	"time"
	"tw_eth_parser/api"
	"tw_eth_parser/ethclient"
	ethparser "tw_eth_parser/ethparser"
	"tw_eth_parser/httpsrv"
	"tw_eth_parser/storage"
)

type Config struct {
	EthURL                  string
	BlockProcessingInterval time.Duration
	EthAPITimeout           time.Duration
	HTTPServerPort          int
}

func main() {
	conf := Config{
		EthURL:                  "https://cloudflare-eth.com",
		BlockProcessingInterval: 15 * time.Second,
		EthAPITimeout:           10 * time.Second,
		HTTPServerPort:          8080,
	}

	ctx := context.Background()
	defer ctx.Done()

	ts := storage.NewRAMTransactionStorage()

	ss := storage.NewRAMSubscribersStorage()

	ec := ethclient.NewEthAPIClient(ethclient.Config{
		EthURL:  conf.EthURL,
		TimeOut: conf.EthAPITimeout,
	})
	pConf := ethparser.Config{
		EthURL:                  conf.EthURL,
		BlockProcessingInterval: conf.BlockProcessingInterval,
	}
	ethParser := ethparser.NewEthParserService(pConf, ec, ts, ss)

	err, cancel := ethParser.Run(ctx)
	defer cancel()

	if err != nil {
		log.Fatalf("Error running eth parser: %v", err)
	}

	handler := api.NewAPIHandler(ethParser)

	server := httpsrv.NewHTTPServer(handler, conf.HTTPServerPort)

	if err := server.Start(); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
