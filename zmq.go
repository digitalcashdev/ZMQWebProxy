package zmqwebproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-zeromq/zmq4"
)

type Subscriber struct {
	ctx         context.Context
	topics      []string
	zmqEndpoint string
	zsub        zmq4.Socket // misnomer, it's semantically the Subscription type
	msgChan     chan Message
	done        chan struct{}
}

type Message struct {
	Event   string
	Raw     []byte
	Counter int
	// Frames  [][]byte
}

func NewChatServer(ctx context.Context, zmqEndpoint string, config Config) *Subscriber {
	s := &Subscriber{
		ctx:         ctx,
		zmqEndpoint: zmqEndpoint,
		topics:      config.Topics,
		zsub:        zmq4.NewSub(ctx),
		msgChan:     make(chan Message),
		done:        make(chan struct{}),
	}
	return s
}

func (s *Subscriber) Connect() error {
	fmt.Fprintf(os.Stderr, "DEBUG: (re)connecting to %s...\n", s.zmqEndpoint)
	err := s.zsub.Dial(s.zmqEndpoint)
	if err != nil {
		return err
	}

	// _ = sub.SetOption(zmq4.OptionSubscribe, "") // subscribe to all topics

	topics := s.topics
	// topics = append(topics, "doesntexist") // arbitrary strings won't error
	// topics = append(topics, "rawtx")
	// topics := []string{"rawtx"}
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if len(topic) == 0 || strings.HasPrefix(topic, "#") || strings.HasPrefix(topic, "//") || strings.HasPrefix(topic, "/*") {
			fmt.Fprintf(os.Stderr, "[ZMQ] ignoring comment '%s'\n", topic)
			continue
		}
		if err := s.zsub.SetOption(zmq4.OptionSubscribe, topic); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[ZMQ] subscribed to topic '%s'\n", topic)
	}

	return nil
}

func (s *Subscriber) ConnectWithReconnect() {
	for {
		err := s.Connect()
		if err != nil {
			s.Close()
			fmt.Fprintf(os.Stderr, "[ZMQ] connection failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "[ZMQ] trying again in 5s...\n")
			time.Sleep(5 * time.Second) // TODO sleep, retry
			continue
		}

		fmt.Fprintf(os.Stderr, "[ZMQ] Waiting for message...\n")
		for {
			msg, err := s.zsub.Recv()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[ZMQ] receive failed: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "[ZMQ] event %s\n", m.Event)
			s.msgChan <- m
		}
	}
}

func (s *Subscriber) Recv() (Message, error) {
	select {
	case msg := <-s.msgChan:
		return msg, nil
	case <-s.done:
		return Message{}, io.EOF
	}
}

func (s *Subscriber) Close() error {
	s.zsub.Close()
	s.done <- struct{}{}
	return nil
}
