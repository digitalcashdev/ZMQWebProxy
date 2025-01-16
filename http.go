package zmqwebproxy

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
)

var (
	roomsByName sync.Map
	clientsByID sync.Map
	msgChan     chan *ChatMessage
	KnownTopics = []string{
		// https://docs.dash.org/en/stable/docs/core/api/zmq.html
		"hashblock",
		"hashchainlock",
		"hashtx",
		"hashtxlock",
		"hashgovernancevote",
		"hashgovernanceobject",
		"hashinstantsenddoublespend",
		"hashrecoveredsig",
		"rpcblock", // TODO add if "rawblock" is present
		"rawblock",
		"rawchainlock",
		"rawchainlocksig",
		"rpctx", // TODO add if "rawtx" is present
		"rawtx",
		"rawtxlock",
		"rawtxlocksig",
		"rpcgovernancevote", // TODO add if "rawgovernancevote" is present
		"rawgovernancevote",
		"rpcgovernanceobject", // TODO add if "rawgovernanceobject" is present
		"rawgovernanceobject",
		"rawinstantsenddoublespend",
		"rawrecoveredsig",
		"sequence",
	}
	KnownTopicList = strings.Join(KnownTopics, ", ")
)

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

func (c *ChatClient) Send(id string, event string, messages []string) {
	if len(id) > 0 {
		_, _ = fmt.Fprintf(c.w, "id: %s\n", id)
	}

	if len(event) > 0 {
		_, _ = fmt.Fprintf(c.w, "event: %s\n", id)
	}

	for _, message := range messages {
		if _, err := fmt.Fprintf(c.w, "data: %s\n", message); err != nil {
			clientsByID.Delete(id) // don't try again
			return
		}
	}

	fmt.Fprintf(c.w, "\n")
	c.rc.Flush()
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

func SendToAll(event string, raw []byte) {
	room := getRoom(event)
	if room == nil {
		return // TODO log sanity fail error
	}

	msgs := []string{}
	switch event {
	case "rpcblock":
		// TODO
	case "rawtx":
		var hasAny bool
		room2 := getRoom(event)
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
				defer sendToAllJSON("rpctx", txJSON)
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
		client.Send(msgID, event, msgs)
		return true
	})
}

func sendToAllJSON(event string, msg []byte) {
	room := getRoom(event)
	if room == nil {
		return // TODO log error
	}

	msgs := []string{}
	msgs = append(msgs, string(msg))

	room.ClientsById.Range(func(k any, v any) bool {
		// clientID := k.(string)
		client := v.(*ChatClient)
		msgID := ""
		client.Send(msgID, event, msgs)
		return true
	})
}

func getClient(name string) *ChatClient {
	if val, ok := clientsByID.Load(name); ok {
		client, _ := val.(*ChatClient) // cannot fail, we exclusively control the map
		return client
	}
	return nil
}

func getRoom(name string) *ChatRoom {
	if val, ok := roomsByName.Load(name); ok {
		room, _ := val.(*ChatRoom) // cannot fail, we exclusively control the map
		return room
	}
	return nil
}

func InitRoutes(mux Muxer) {
	for _, name := range KnownTopics {
		room := &ChatRoom{
			ClientsById:    sync.Map{},
			RecentMessages: []ChatMessage{},
		}
		roomsByName.Store(name, room)
	}

	mux.HandleFunc("GET /api/version", versionHandler)
	mux.HandleFunc("GET /api/hello", helloHandler)

	mux.HandleFunc("GET /api/notify/{id}", notifyPublishHandler)
	mux.HandleFunc("POST /api/notify/{id}", notifyUpdateHandler)
	mux.HandleFunc("PUT /api/notify/{id}", notifySetHandler)
	mux.HandleFunc("DELETE /api/notify/{id}", notifyRemoveHandler)
	mux.HandleFunc("/api/notify/", methodNotAllowedHandler) // handle trailing slash
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"version": "0.1.0"})
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "hello"})
}

func notifyPublishHandler(w http.ResponseWriter, r *http.Request) {
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
	clientsByID.Store(id, client)
	defer clientsByID.Delete(id)

	// err := subscribe(id, requestedTopics)
	// if err != nil {
	// 	EndWithError(w, http.StatusBadRequest, err.Error())
	// 	return
	// }

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	<-r.Context().Done()
}

func notifyUpdateHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := subscribe(id, options.Topics); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	var topics []string
	roomsByName.Range(func(k any, v any) bool {
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

func notifySetHandler(w http.ResponseWriter, r *http.Request) {
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
	roomsByName.Range(func(k any, v any) bool {
		name := k.(string)
		room := v.(*ChatRoom)

		if _, ok := room.ClientsById.LoadAndDelete(id); ok {
			oldTopics = append(oldTopics, name)
		}
		return true
	})

	if err := subscribe(id, options.Topics); err != nil {
		EndWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	topicList := strings.Join(oldTopics, ", ")
	EndWithResult(w, http.StatusOK, fmt.Sprintf("debug: replaced subscriptions: '%s'", topicList))
}

func notifyRemoveHandler(w http.ResponseWriter, r *http.Request) {
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
	roomsByName.Range(func(k any, v any) bool {
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

func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func subscribe(id string, topics []string) error {
	nonTopics := getUniqueToRight(KnownTopics, topics)
	if len(nonTopics) > 0 {
		err := fmt.Errorf(
			"unknown events: '%s', valid events are '%s'",
			strings.Join(nonTopics, ", "),
			strings.Join(KnownTopics, ", "),
		)
		return err
	}

	client := getClient(id)
	if client == nil {
		err := fmt.Errorf("'%s' is not a current client", id)
		return err
	}

	for _, topic := range topics {
		room := getRoom(topic)
		room.ClientsById.Store(id, client)
	}

	return nil
}

func parseDelimitedString(fieldList string /*, delimiter string*/) []string {
	f := func(c rune) bool {
		return ',' == c || !unicode.IsSpace(c)
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
