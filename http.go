package zmqwebproxy

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Server is the API Service
type Server struct {
	Config      Config
	roomsByName sync.Map
	clientsByID sync.Map
	msgChan     chan *ChatMessage
}

// EventSourceMessage goes to the client via channel
type EventSourceMessage struct {
	ID    string   `json:"id"`
	Event string   `json:"event"`
	Data  []string `json:"data"`
}

// Config is configuration
type Config struct {
	Topics []string `json:"topics"`
}

// ChatRoom holds a map of clients subscribed to it.
type ChatRoom struct {
	count          atomic.Int64
	ClientsById    sync.Map // Key: RemoteAddr (string), Value: *Client
	RecentMessages []ChatMessage
}

// ChatClient represents a connected client, and some metadata.
type ChatClient struct {
	id       string
	r        *http.Request
	w        http.ResponseWriter
	rc       *http.ResponseController
	messages chan EventSourceMessage
}

func (s *Server) NewEventSourceClient(id string, w http.ResponseWriter, r *http.Request) *ChatClient {
	// A block can contain ~10,000 transactions (2-megabyte / 200-byte),
	// which only happens about every two minutes.
	// TODO We'll need to debounce the Flush.
	bufferSize := 1000
	client := &ChatClient{
		id:       id,
		r:        r,
		w:        w,
		rc:       http.NewResponseController(w),
		messages: make(chan EventSourceMessage, bufferSize),
	}
	s.clientsByID.Store(client.id, client)
	return client
}

// Send puts the message in a channel to the client
func (c *ChatClient) Send(msg EventSourceMessage) error {
	select {
	case c.messages <- msg:
		return nil
	default:
		return fmt.Errorf("client is writing too slow")
	}
}

func (c *ChatClient) WriteEventSourceHeaders() {
	c.w.Header().Set("Content-Type", "text/event-stream")
	c.w.Header().Set("Cache-Control", "no-cache")
	c.w.WriteHeader(http.StatusOK)
	c.rc.Flush()
}

// RelayEventsUntilClose reads from the messages channel until the client is done
func (c *ChatClient) RelayEventsUntilClose() {
	for {
		select {
		case <-c.r.Context().Done():
			return
		case msg := <-c.messages:
			c.EventSourceSend(msg)
		}
	}
}

func (c *ChatClient) EventSourceSend(msg EventSourceMessage) {
	if len(msg.ID) > 0 {
		_, _ = fmt.Fprintf(c.w, "id: %s\n", msg.ID)
	}

	if len(msg.Event) > 0 {
		_, _ = fmt.Fprintf(c.w, "event: %s\n", msg.Event)
	}

	if len(msg.Data) == 0 {
		_, _ = fmt.Fprintf(c.w, "data: \n")
	} else {
		for _, data := range msg.Data {
			_, _ = fmt.Fprintf(c.w, "data: %s\n", data)
		}
	}

	_, _ = fmt.Fprintf(c.w, "\n")
	c.Flush()
}

// Wraps http.ResponseController's Flush to avoid after-handler panics
// "A ResponseController may not be used after the [Handler.ServeHTTP]
// method has returned."
// See https://github.com/golang/go/issues/19959#issuecomment-293933806
func (c *ChatClient) Flush() {
	c.rc.Flush()
}

// CloseClient cleans up the client, including closing the channel
func (s *Server) CloseClient(c *ChatClient) error {
	s.roomsByName.Range(func(k any, v any) bool {
		room := v.(*ChatRoom)
		room.ClientsById.Delete(c.id)
		return true
	})
	s.clientsByID.Delete(c.id)
	close(c.messages)
	return nil
}

// ChatClient represents a message
type ChatMessage struct {
	Topic     string
	Raw       []byte
	Timestamp time.Time
}

type ChatSubscriptionOptions struct {
	Topics []string `json:"topics"`
}

type Muxer interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type ResultResponse struct {
	Result string `json:"result"`
}

func EndWithError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	errorResponse := ErrorResponse{
		Error: message,
	}
	_ = json.NewEncoder(w).Encode(errorResponse)
}

func EndWithResult(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	response := ResultResponse{
		Result: message,
	}
	_ = json.NewEncoder(w).Encode(response)
}

func New(config Config) *Server {
	s := &Server{
		Config: config,
	}

	for _, name := range s.Config.Topics {
		room := &ChatRoom{
			ClientsById:    sync.Map{},
			RecentMessages: []ChatMessage{},
		}
		s.roomsByName.Store(name, room)
	}

	return s
}

func (s *Server) SendToAll(event string, raw []byte) {
	msgStr := hex.EncodeToString(raw)
	// msgStr := base64.StdEncoding.EncodeToString(raw)

	room := s.getRoom(event)
	if room == nil {
		fmt.Fprintf(os.Stderr, "[sanity fail] zmq: no room %q\n", event)
		return
	}

	var hasAny bool
	room.ClientsById.Range(func(k any, v any) bool {
		hasAny = true
		return false
	})
	if !hasAny {
		fmt.Fprintf(os.Stderr, "[skip] zmq %q: no subscribers to topic\n", event)
		return
	}

	var result string
	switch event {
	case "rawblock":
		if block, err := ParseBlock(raw); err != nil {
			// ignore junk data, nothing we can do anyway
			fmt.Fprintf(os.Stderr, "[error] zmq: couldn't parse block: %s\n%x\n", err, raw)
		} else {
			block.Raw = raw
			jsonBytes, _ := json.Marshal(block)
			result = string(jsonBytes)
		}
	case "rawtx":
		if tx, err := ParseTx(raw); err != nil {
			// ignore junk data, nothing we can do anyway
			fmt.Fprintf(os.Stderr, "[error] zmq: couldn't parse tx: %s\n%x\n", err, raw)
		} else {
			tx.Raw = raw
			jsonBytes, _ := json.Marshal(tx)
			result = string(jsonBytes)
		}
	case "rawgovernancevote":
		// TODO
	case "rawgovernanceobject":
		// TODO
	default:
		// ignore
	}

	msgs := []string{}
	if len(result) == 0 {
		result = fmt.Sprintf(`{"raw":"%s"}`, msgStr)
	}
	msgs = append(msgs, result)

	count := room.count.Add(1)
	lastEventID := strconv.FormatInt(count, 10)
	room.ClientsById.Range(func(k any, v any) bool {
		// clientID := k.(string)
		client := v.(*ChatClient)
		if client == nil {
			fmt.Fprintf(os.Stderr, "SANITY FAIL: client disappeared\n")
			return true
		}
		_ = client.Send(EventSourceMessage{
			ID:    lastEventID,
			Event: event,
			Data:  msgs,
		})
		return true
	})
}

func (s *Server) getClient(name string) *ChatClient {
	if val, ok := s.clientsByID.Load(name); ok {
		client, _ := val.(*ChatClient) // cannot fail, we exclusively control the map
		return client
	}
	return nil
}

func (s *Server) getRoom(name string) *ChatRoom {
	if val, ok := s.roomsByName.Load(name); ok {
		room, _ := val.(*ChatRoom) // cannot fail, we exclusively control the map
		return room
	}
	return nil
}

func (s *Server) TopicsListHandler(w http.ResponseWriter, r *http.Request) {
	var topics []string
	for _, topic := range s.Config.Topics {
		if len(topic) == 0 ||
			strings.HasPrefix(topic, "#") ||
			strings.HasPrefix(topic, "//") ||
			strings.HasPrefix(topic, "/*") {
			continue
		}
		topics = append(topics, topic)
	}
	EndWithResult(w, http.StatusOK, strings.Join(topics, ", "))
}

func (s *Server) NotifyPublishHandler(w http.ResponseWriter, r *http.Request) {
	r.Body.Close()

	id := r.PathValue("id")
	if len(id) == 0 {
		uuidv7, _ := uuid.NewV7()
		id = uuidv7.String()
	} else {
		_, err := uuid.Parse(id)
		if err != nil {
			EndWithError(w, http.StatusBadRequest, "'id' must be a valid UUIDv4")
			return
		}
	}

	client := s.NewEventSourceClient(id, w, r)

	requestedTopics := parseDelimitedString(r.URL.Query().Get("dbg_topics")) // dbg for curl only due to errors
	fmt.Fprintf(os.Stderr, "[DEBUG] requestedTopics %s %s\n", id, requestedTopics)
	if err := s.subscribe(id, requestedTopics); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	client.WriteEventSourceHeaders()
	client.RelayEventsUntilClose()

	fmt.Fprintf(os.Stderr, "[DEBUG] closing client %s\n", id)
	_ = s.CloseClient(client)
}

func (s *Server) NotifyUpdateHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	id := r.PathValue("id")
	_, err := uuid.Parse(id)
	if err != nil {
		EndWithError(w, http.StatusBadRequest, "'id' must be a valid UUIDv4")
		return
	}

	var options ChatSubscriptionOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.subscribe(id, options.Topics); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	var topics []string
	s.roomsByName.Range(func(k any, v any) bool {
		name := k.(string)
		room := v.(*ChatRoom)

		if _, ok := room.ClientsById.Load(id); ok {
			topics = append(topics, name)
		}
		return true
	})

	topicList := strings.Join(topics, ", ")
	EndWithResult(w, http.StatusOK, fmt.Sprintf("debug: subscriptions: '%s'", topicList))
}

func (s *Server) NotifySetHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	id := r.PathValue("id")
	_, err := uuid.Parse(id)
	if err != nil {
		EndWithError(w, http.StatusBadRequest, "'id' must be a valid UUIDv4")
		return
	}

	var options ChatSubscriptionOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	var oldTopics []string
	s.roomsByName.Range(func(k any, v any) bool {
		name := k.(string)
		room := v.(*ChatRoom)

		if _, ok := room.ClientsById.LoadAndDelete(id); ok {
			oldTopics = append(oldTopics, name)
		}
		return true
	})

	if err := s.subscribe(id, options.Topics); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	topicList := strings.Join(oldTopics, ", ")
	EndWithResult(w, http.StatusOK, fmt.Sprintf("debug: replaced subscriptions: '%s'", topicList))
}

func (s *Server) NotifyRemoveHandler(w http.ResponseWriter, r *http.Request) {
	r.Body.Close()

	id := r.PathValue("id")
	_, err := uuid.Parse(id)
	if err != nil {
		EndWithError(w, http.StatusBadRequest, "'id' must be a valid UUIDv4")
		return
	}

	var options ChatSubscriptionOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	var oldTopics []string
	s.roomsByName.Range(func(k any, v any) bool {
		name := k.(string)
		room := v.(*ChatRoom)

		if _, ok := room.ClientsById.LoadAndDelete(id); ok {
			oldTopics = append(oldTopics, name)
		}
		return true
	})

	topicList := strings.Join(oldTopics, ", ")
	EndWithResult(w, http.StatusOK, fmt.Sprintf("debug: removed subscriptions: '%s'", topicList))
}

func (s *Server) subscribe(id string, topics []string) error {
	nonTopics := getUniqueToRight(s.Config.Topics, topics)
	if len(nonTopics) > 0 {
		err := fmt.Errorf(
			"unknown events: '%s', valid events are '%s'",
			strings.Join(nonTopics, ", "),
			strings.Join(s.Config.Topics, ", "),
		)
		return err
	}

	client := s.getClient(id)
	if client == nil {
		// TODO create client info before client connects (and cleanup on timeout)
		err := fmt.Errorf("'%s' is not a current client", id)
		return err
	}

	for _, topic := range topics {
		room := s.getRoom(topic)
		room.ClientsById.Store(client.id, client)
	}

	return nil
}

func parseDelimitedString(fieldList string /*, delimiter string*/) []string {
	f := func(c rune) bool {
		return ',' == c || unicode.IsSpace(c)
	}
	fields := strings.FieldsFunc(fieldList, f)
	return fields
}

func getUniqueToRight(leftHaystack, rightNeedles []string) []string {
	sharedSet := make(map[string]struct{})

	for _, room := range leftHaystack {
		sharedSet[room] = struct{}{}
	}

	var uniqueList []string

	for _, needle := range rightNeedles {
		if _, exists := sharedSet[needle]; !exists {
			uniqueList = append(uniqueList, needle)
		}
	}

	return uniqueList
}
