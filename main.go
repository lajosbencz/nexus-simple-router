package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/router"
	"github.com/gammazero/nexus/v3/wamp"
)

var (
	realm       = "default"
	wsHost      = "localhost"
	wsPort      = 8951
	localClient *client.Client
	logger      *log.Logger
	devEcho     = true
)

func main() {

	flag.StringVar(&realm, "realm", "default", "Realm to be created")
	flag.StringVar(&wsHost, "host", "localhost", "Host to listen on")
	flag.IntVar(&wsPort, "port", 8951, "Port to listen on")
	flag.BoolVar(&devEcho, "decho", true, "Should dev.echo RPC be registered")
	flag.Parse()
	wsAddr := fmt.Sprintf("%s:%d", wsHost, wsPort)

	logger = log.New(os.Stdout, "", log.LstdFlags)

	routerConfig := &router.Config{
		RealmConfigs: []*router.RealmConfig{
			{
				URI:           wamp.URI(realm),
				AnonymousAuth: true,
				AllowDisclose: true,
			},
		},
	}

	wsRouter, err := router.NewRouter(routerConfig, logger)
	if err != nil {
		panic(err)
	}
	defer wsRouter.Close()

	clientConfig := client.Config{
		Realm:  realm,
		Logger: logger,
	}
	localClient, err = client.ConnectLocal(wsRouter, clientConfig)
	if err != nil {
		panic(err)
	}
	defer localClient.Close()

	wsServer := router.NewWebsocketServer(wsRouter)
	wsServer.Upgrader.EnableCompression = true
	wsServer.EnableTrackingCookie = true
	wsServer.KeepAlive = 30 * time.Second

	wsCloser, err := wsServer.ListenAndServe(wsAddr)
	if err != nil {
		panic(err)
	}
	defer wsCloser.Close()

	if devEcho {
		err = createLocalCallee(localClient, "dev.echo", func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
			time.Sleep(2 * time.Second)
			return client.InvokeResult{
				Args:   inv.Arguments,
				Kwargs: inv.ArgumentsKw,
			}
		})
		if err != nil {
			panic(err)
		}
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt)

	logger.Printf("listening on ws://%s\n", wsAddr)
	<-shutdown
}

func createLocalCallee(client *client.Client, procedure string, callback func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult) error {
	if err := client.Register(procedure, callback, nil); err != nil {
		return fmt.Errorf("failed to register %q: %s", procedure, err)
	}
	logger.Printf("registered RPC: %s\n", procedure)
	return nil
}
