package zmqwebproxy

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Server is the API Service
type Server struct {
	Config      Config
	roomsByName sync.Map
	clientsByID sync.Map
	msgChan     chan *ChatMessage
}

// Config is configuration
type Config struct {
	Topics []string `json:"topics"`
}

// ChatRoom holds a map of clients subscribed to it.
type ChatRoom struct {
	ClientsById    sync.Map // Key: RemoteAddr (string), Value: *Client
	RecentMessages []ChatMessage
}

// ChatClient represents a connected client, and some metadata.
type ChatClient struct {
	w  http.ResponseWriter
	rc *http.ResponseController
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
	Result string `json:"error"`
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

func (s *Server) Send(c *ChatClient, id string, event string, messages []string) {
	if len(id) > 0 {
		_, _ = fmt.Fprintf(c.w, "id: %s\n", id)
	}

	if len(event) > 0 {
		_, _ = fmt.Fprintf(c.w, "event: %s\n", id)
	}

	for _, message := range messages {
		if _, err := fmt.Fprintf(c.w, "data: %s\n", message); err != nil {
			s.clientsByID.Delete(id) // don't try again
			return
		}
	}

	fmt.Fprintf(c.w, "\n")
	c.rc.Flush()
}

func (s *Server) SendToAll(event string, raw []byte) {
	room := s.getRoom(event)
	if room == nil {
		return // TODO log sanity fail error
	}

	msgs := []string{}
	switch event {
	case "rpcblock":
		// TODO
	case "rawtx":
		var hasAny bool
		room2 := s.getRoom(event)
		room2.ClientsById.Range(func(k any, v any) bool {
			hasAny = true
			return false
		})
		if hasAny {
			// TODO does the client want all messages?
			if tx, err := parseTx(raw); err != nil {
				// ignore junk data, nothing we can do anyway
			} else {
				// txJSON, _ = json.MarshalIndent(tx, "", "  ")
				txJSON, _ := json.Marshal(tx)
				defer s.sendToAllJSON("rpctx", txJSON)
			}
		}
	case "rpcgovernancevote":
		// TODO
	case "rpcgovernanceobject":
		// TODO
	default:
		// ignore
	}

	msgHex := hex.EncodeToString(raw)
	msgs = append(msgs, msgHex)

	room.ClientsById.Range(func(k any, v any) bool {
		// clientID := k.(string)
		client := v.(*ChatClient)
		msgID := ""
		s.Send(client, msgID, event, msgs)
		return true
	})
}

func (s *Server) sendToAllJSON(event string, msg []byte) {
	room := s.getRoom(event)
	if room == nil {
		return // TODO log error
	}

	msgs := []string{}
	msgs = append(msgs, string(msg))

	room.ClientsById.Range(func(k any, v any) bool {
		// clientID := k.(string)
		client := v.(*ChatClient)
		msgID := ""
		s.Send(client, msgID, event, msgs)
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

func (s *Server) NotifyPublishHandler(w http.ResponseWriter, r *http.Request) {
	r.Body.Close()

	id := r.PathValue("id")
	_, err := uuid.Parse(id)
	if err != nil {
		EndWithError(w, http.StatusBadRequest, "'id' must be a valid UUIDv4")
		return
	}

	// requestedTopics, err := parseDelimitedString(r.Query.Get("topics"))
	// if err != nil {
	// 	// TODO maybe only allow this via POST so that the user gets the JSON
	// 	EndWithError(w, http.StatusBadRequest, "'topics' must be a comma- or space-delimited list of")
	// 	return
	// }

	client := &ChatClient{
		w:  w,
		rc: http.NewResponseController(w),
	}
	s.clientsByID.Store(id, client)
	defer s.clientsByID.Delete(id)

	// err := subscribe(id, requestedTopics)
	// if err != nil {
	// 	EndWithError(w, http.StatusBadRequest, err.Error())
	// 	return
	// }

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	<-r.Context().Done()
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
		err := fmt.Errorf("'%s' is not a current client", id)
		return err
	}

	for _, topic := range topics {
		room := s.getRoom(topic)
		room.ClientsById.Store(id, client)
	}

	return nil
}

// func parseDelimitedString(fieldList string /*, delimiter string*/) []string {
// 	f := func(c rune) bool {
// 		return ',' == c || !unicode.IsSpace(c)
// 	}
// 	fields := strings.FieldsFunc(fieldList, f)
// 	return fields
// }

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
