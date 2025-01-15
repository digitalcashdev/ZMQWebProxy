package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"time"

	"github.com/go-zeromq/zmq4"
)

type Subscriber struct {
	ctx         context.Context
	zmqEndpoint string
	zsub        zmq4.Socket // misnomer, it's semantically the Subscription type
	msgChan     chan Message
	// done        chan struct{}
}

type Message struct {
	Event   string
	Raw     []byte
	Counter int
	// Frames  [][]byte
}

func NewChatServer(ctx context.Context, zmqEndpoint string) *Subscriber {
	zsub := zmq4.NewSub(ctx)
	s := &Subscriber{
		ctx:         ctx,
		zmqEndpoint: zmqEndpoint,
		zsub:        zsub,
		msgChan:     make(chan Message),
		// done:        make(chan struct{}),
	}
	return s
}

func (s *Subscriber) Connect() error {
	zsub := zmq4.NewSub(context.Background())
	fmt.Printf("Connecting to %s...\n", s.zmqEndpoint)
	err := zsub.Dial(s.zmqEndpoint)
	if err != nil {
		return err
	}

	// _ = sub.SetOption(zmq4.OptionSubscribe, "") // subscribe to all topics

	topics := []string{
		"doesntexist", // arbitrary strings won't error
		"rawtx",       // documented as 'zmqpubdoesntexist'
	}
	for _, topic := range topics {
		fmt.Printf("Subscribing to topic '%s'\n", topic)
		if err := s.zsub.SetOption(zmq4.OptionSubscribe, topic); err != nil {
			return err
		}
	}

	return nil
}

func (s *Subscriber) ConnectWithReconnect() {
	for {
		err := s.Connect()
		if err != nil {
			s.Close()
			fmt.Fprintf(os.Stderr, "connection failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "trying again in 5s...\n")
			time.Sleep(5 * time.Second) // TODO sleep, retry
			continue
		}

		for {
			msg, err := s.zsub.Recv()
			if err != nil {
				fmt.Fprintf(os.Stderr, "receive failed: %v\n", err)
				break
			}

			event := string(msg.Frames[0])
			raw := msg.Frames[1]
			ucounter := binary.LittleEndian.Uint32(msg.Frames[2])
			counter := int(ucounter)
			// frames := msg.Frames[1:]

			m := Message{
				Event:   event,
				Raw:     raw,
				Counter: counter,
				// Frames:  frames,
			}
			s.msgChan <- m
		}
	}
}

func (s *Subscriber) Recv() Message {
	return <-s.msgChan
}

func (s *Subscriber) Close() {
	s.zsub.Close()
}
