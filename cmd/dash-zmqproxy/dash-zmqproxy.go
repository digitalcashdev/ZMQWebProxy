package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/DigitalCashDev/zmqwebproxy"
	"github.com/DigitalCashDev/zmqwebproxy/internal"
	"github.com/DigitalCashDev/zmqwebproxy/static"

	"github.com/go-zeromq/zmq4"
	"github.com/joho/godotenv"
)

var (
	name = "goboilerplate"
	// these will be replaced by goreleaser
	version = "0.0.0-dev"
	date    = "0001-01-01T00:00:00Z"
	commit  = "0000000"
)

var config zmqwebproxy.Config

func printVersion() {
	// go run
	fmt.Printf("%s v%s %s (%s)\n", name, version, commit[:7], date)
	fmt.Printf("Copyright (C) 2025 AJ ONeal\n")
	fmt.Printf("Licensed under the MPL-2.0 license\n")
}

func main() {
	var subcmd string
	var envPath string
	var httpPort int

	nArgs := len(os.Args)
	if nArgs >= 2 {
		opt := os.Args[1]
		subcmd = strings.TrimPrefix(opt, "-")
		if opt == "-V" || subcmd == "version" {
			printVersion()
			os.Exit(0)
			return
		}
	}

	{
		envPath = peekOption(os.Args, []string{"--env", "-env"})
		if len(envPath) > 0 {
			fmt.Fprintf(os.Stderr, "reading ENVs from %s", envPath)
			if err := godotenv.Load(envPath); err != nil {
				fmt.Fprintf(os.Stderr, ": skipped (%s)", err.Error())
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
	}

	defaultHTTPPort := 8080
	httpPortStr := os.Getenv("PORT")
	if len(httpPortStr) > 0 {
		defaultHTTPPort, _ = strconv.Atoi(httpPortStr)
		if defaultHTTPPort == 0 {
			defaultHTTPPort = 8080
		}
	}

	defaultConfigJSONPath := "public-config.json"

	defaultDashZMQHost := "tcp://127.0.0.1:28332"
	dashZMQHost := os.Getenv("DASHD_ZMQ_HOST")
	if len(dashZMQHost) > 0 {
		defaultDashZMQHost = dashZMQHost
	}

	configJSONPath := defaultConfigJSONPath
	overlayFS := &internal.OverlayFS{}
	flag.StringVar(&configJSONPath, "config", defaultConfigJSONPath, "JSON config path, relative to ./static/, ex: ./config.json")
	flag.StringVar(&envPath, "env", "", "load ENVs from file, ex: ./.env")
	flag.StringVar(&dashZMQHost, "zmq-host", defaultDashZMQHost, "dashd ZMQ address and port")
	flag.StringVar(&overlayFS.WebRoot, "web-root", "./public/", "serve from the given directory")
	flag.BoolVar(&overlayFS.WebRootOnly, "web-root-only", false, "do not serve the embedded web root")
	flag.IntVar(&httpPort, "port", defaultHTTPPort, "bind and listen for http on this port")
	flag.Parse()
	if subcmd == "help" {
		flag.Usage()
		os.Exit(0)
		return
	}

	if !strings.HasPrefix(dashZMQHost, "tcp:") {
		dashZMQHost = fmt.Sprintf("tcp://%s", dashZMQHost)
	}
	overlayFS.LocalFS = http.Dir(overlayFS.WebRoot)
	overlayFS.EmbedFS = http.FS(static.FS)

	f, err := overlayFS.ForceLocalOrEmbedOpen(configJSONPath)
	if err != nil {
		log.Fatalf("loading RPC JSON description file '%s' failed: %v", configJSONPath, err)
	}

	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&config); err != nil {
		log.Fatalf("decoding %s failed: %v", configJSONPath, err)
		return
	}

	// targetAddress := os.Args[1] // ex: "XfA3nC6Bw3bP5mR9cJ4FQHvDdNq6FyLz5V"
	// pkhBytes, verByte, err := base58.CheckDecode(targetAddress)
	// if err != nil {
	// 	log.Fatalf("%s", err)
	// }
	// // pkhHex := hex.EncodeToString(pkhBytes)
	// // verHex := hex.EncodeToString([]byte{version})
	// // TODO check version
	// fmt.Printf("address (version + pkh): 0x%x 0x%x\n", verByte, pkhBytes)

	srv := zmqwebproxy.New(config)

	go func() {
		// if the very first connection fails, let the user know
		// TODO send back an error via the API instead if it's not in a ready state
		zsub := zmq4.NewSub(context.Background())
		fmt.Printf("Connecting to %s...\n", dashZMQHost)
		err := zsub.Dial(dashZMQHost)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect: %v", err)
			os.Exit(1)
		}

		s := zmqwebproxy.NewChatServer(context.TODO(), dashZMQHost, srv.Config)
		s.ConnectWithReconnect()
		for {
			msg := s.Recv()
			srv.SendToAll(msg.Event, msg.Raw)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("OPTIONS /", zmqwebproxy.AddCORSHandler)

	fileServer := http.FileServer(overlayFS)
	mux.Handle("GET /", fileServer)

	mux.HandleFunc("GET /api/version", zmqwebproxy.CORSMiddleware(versionHandler))
	mux.HandleFunc("GET /api/hello", helloHandler)

	mux.HandleFunc("GET /api/notify/{id}", srv.NotifyPublishHandler)
	mux.HandleFunc("POST /api/notify/{id}", srv.NotifyUpdateHandler)
	mux.HandleFunc("PUT /api/notify/{id}", srv.NotifySetHandler)
	mux.HandleFunc("DELETE /api/notify/{id}", srv.NotifyRemoveHandler)
	// mux.HandleFunc("/api/notify/", methodNotAllowedHandler) // handle trailing slash

	log.Printf("Server is running on http://localhost%d", httpPort)
	bindAddr := fmt.Sprintf(":%d", httpPort)
	if err := http.ListenAndServe(bindAddr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func peekOption(args, aliases []string) string {
	var flagIndex int

	for _, alias := range aliases {
		flagIndex = slices.Index(args, alias)
		if flagIndex > -1 {
			break
		}
	}

	if flagIndex == -1 {
		return ""
	}

	argIndex := flagIndex + 1
	if len(args) <= argIndex {
		return ""
	}

	return args[argIndex]
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	result := struct {
		Version string `json:"version"`
	}{
		Version: version,
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(result)
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if len(name) == 0 {
		name = "World"
	}
	result := struct {
		Message string `json:"message"`
	}{
		Message: fmt.Sprintf("Hello, %s!", name),
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(result)
}

// func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
// 	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// }
