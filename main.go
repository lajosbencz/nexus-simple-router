package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/router"
	"github.com/gammazero/nexus/v3/wamp"
)

var (
	realm       = "default"
	wsEnable    = true
	wsHost      = "localhost"
	wsPort      = 8951
	rsEnable    = true
	rsHost      = "127.0.0.1"
	rsPort      = 8952
	rsProto     = "tcp"
	localClient *client.Client
	logger      *log.Logger
	devEcho     = false
	devTime     = false
)

func main() {

	flag.StringVar(&realm, realm, "default", "Realm to be created")
	flag.BoolVar(&wsEnable, "ws", wsEnable, "Should WebSocket transport be started")
	flag.StringVar(&wsHost, "ws-host", wsHost, "WebSocket host to listen on")
	flag.IntVar(&wsPort, "ws-port", wsPort, "WebSocket port to listen on")
	flag.BoolVar(&rsEnable, "rs", rsEnable, "Should RawSocket transport be started")
	flag.StringVar(&rsHost, "rs-host", rsHost, "RawSocket host to listen on")
	flag.IntVar(&rsPort, "rs-port", rsPort, "RawSocket port to listen on")
	flag.StringVar(&rsProto, "rs-proto", rsProto, "RawSocket protocol (tcp,tcp4,tcp6,unix,unixpacket)")
	flag.BoolVar(&devEcho, "decho", devEcho, "Should dev.echo RPC be registered")
	flag.BoolVar(&devTime, "dtime", devTime, "Should the time be regularly published on dev.time")
	flag.Parse()

	if !wsEnable && !rsEnable {
		panic("one of WebSocket (-ws) or RawSocket (-rs) transports must be enabled")
	}

	wsAddr := fmt.Sprintf("%s:%d", wsHost, wsPort)
	rsAddr := fmt.Sprintf("%s:%d", rsHost, rsPort)

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

	if wsEnable {
		wsServer := router.NewWebsocketServer(wsRouter)
		wsServer.Upgrader.EnableCompression = true
		wsServer.Upgrader.CheckOrigin = func(res *http.Request) bool {
			return true
		}
		wsServer.EnableTrackingCookie = true
		wsServer.KeepAlive = 30 * time.Second
		wsCloser, err := wsServer.ListenAndServe(wsAddr)
		if err != nil {
			panic(err)
		}
		defer wsCloser.Close()
		logger.Printf("listening on ws://%s\n", wsAddr)
	}

	if rsEnable {
		rsServer := router.NewRawSocketServer(wsRouter)
		rsServer.KeepAlive = 30 * time.Second
		rsCloser, err := rsServer.ListenAndServe(rsProto, rsAddr)
		if err != nil {
			panic(err)
		}
		defer rsCloser.Close()
		logger.Printf("listening on %s://%s\n", rsProto, rsAddr)
	}

	if devEcho {
		err = createLocalCallee(localClient, "dev.echo", func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult {
			time.Sleep(2 * time.Second)
			res := client.InvokeResult{
				Args:   inv.Arguments,
				Kwargs: inv.ArgumentsKw,
			}
			fmt.Printf("dev.echo %v %v\n", res, inv.Details)
			return res
		})
		if err != nil {
			panic(err)
		}
	}

	if devTime {
		ticker := time.NewTicker(time.Second * 5)
		tickerQuit := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					now := time.Now()
					nowStr := now.Format(time.RFC3339)
					logger.Printf("dev.time: %s\n", nowStr)
					localClient.Publish("dev.time", wamp.Dict{}, wamp.List{nowStr}, wamp.Dict{})
				case <-tickerQuit:
					ticker.Stop()
					return
				}
			}
		}()
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt)

	<-shutdown
}

func createLocalCallee(client *client.Client, procedure string, callback func(ctx context.Context, inv *wamp.Invocation) client.InvokeResult) error {
	if err := client.Register(procedure, callback, nil); err != nil {
		return fmt.Errorf("failed to register %q: %s", procedure, err)
	}
	logger.Printf("registered RPC: %s\n", procedure)
	return nil
}
