package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/DigitalCashDev/zmqwebproxy"

	"github.com/dashpay/dashd-go/btcutil/base58"
	"github.com/go-zeromq/zmq4"
)

var (
	port = ":3000" // Default port, can be overridden by environment variable
)

func showHelp(w io.Writer) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "USAGE")
	fmt.Fprintln(w, "    . ~/.config/dash-zmqproxy/env ; dash-zmqproxy <address>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "EXAMPLE")
	fmt.Fprintln(w, "    . ~/.config/dash-zmqproxy/env ; dash-zmqproxy 'XfA3nC6Bw3bP5mR9cJ4FQHvDdNq6FyLz5V'")
	fmt.Fprintln(w, "")
	// "XfA3nC6Bw3bP5mR9cJ4FQHvDdNq6FyLz5V"
}

func getPort() string {
	// Implement logic to read environment variable for port
	return os.Getenv("PORT")
}

func main() {
	dashCoreZMQEndpoint := os.Getenv("DASH_ZMQ_HOST") // "tcp://127.0.0.1:28332"              // Change this to your Dash Core ZMQ endpoint

	if p := getPort(); p != "" {
		port = ":" + p
	}

	if len(os.Args) < 2 {
		showHelp(os.Stderr)
		os.Exit(1)
	}
	targetAddress := os.Args[1] // ex: "XfA3nC6Bw3bP5mR9cJ4FQHvDdNq6FyLz5V"
	pkhBytes, verByte, err := base58.CheckDecode(targetAddress)
	if err != nil {
		log.Fatalf("%s", err)
	}
	// pkhHex := hex.EncodeToString(pkhBytes)
	// verHex := hex.EncodeToString([]byte{version})
	// TODO check version
	fmt.Printf("address (version + pkh): 0x%x 0x%x\n", verByte, pkhBytes)

	// targetAddresses = append(targetAddresses, targetAddress)

	go func() {
		// if the very first connection fails, let the user know
		// TODO send back an error via the API instead if it's not in a ready state
		zsub := zmq4.NewSub(context.Background())
		fmt.Printf("Connecting to %s...\n", dashCoreZMQEndpoint)
		err := zsub.Dial(dashCoreZMQEndpoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect: %v", err)
			os.Exit(1)
		}

		s := zmqwebproxy.NewChatServer(context.TODO(), dashCoreZMQEndpoint)
		s.ConnectWithReconnect()
		for {
			msg := s.Recv()
			zmqwebproxy.SendToAll(msg.Event, msg.Raw)
		}
	}()

	mux := http.NewServeMux()
	zmqwebproxy.InitRoutes(mux)

	log.Printf("Server is running on http://localhost%s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
