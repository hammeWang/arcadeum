package server

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/horizon-games/arcadeum/server/config"
	"github.com/horizon-games/arcadeum/server/matcher"
)

type Server struct {
	Matcher *matcher.Service
}

type MessageRequest struct {
	PlayerConn *websocket.Conn // sender
	*matcher.Message
}

var relay = make(chan *MessageRequest)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow cross origin connections
	},
}

func New(cfg *config.Config) *Server {
	return &Server{
		Matcher: matcher.NewService(
			&cfg.ENV,
			&cfg.MatcherConfig,
			&cfg.ETHConfig,
			&cfg.ArcadeumConfig,
			&cfg.RedisConfig),
	}
}

func (s *Server) Start() {
	go s.HandleMessages()
	go s.Matcher.HandleMatchResponses()
}

func (s *Server) HandleMessages() {
	for {
		msg := <-relay
		err := s.Matcher.OnMessage(msg.Message)
		if err != nil {
			msg.PlayerConn.WriteJSON(matcher.NewError(err.Error()))
		}
	}
}

func (s *Server) HandleConnections(w http.ResponseWriter, r *http.Request) {
	log.Println("Opening WS connection")
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	s.FindMatch(matcher.Context(r), ws)

	for {
		var msg matcher.Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			ws.WriteJSON(matcher.NewError("Unrecognized message format."))
		} else {
			relay <- &MessageRequest{PlayerConn: ws, Message: &msg}
		}
	}
}

func (s *Server) FindMatch(token *matcher.Token, conn *websocket.Conn) {
	channel := make(chan *matcher.Message)
	s.Matcher.Subscribe(token.SubKey.String(), channel)
	go OnMessage(conn, channel)
	go s.Matcher.FindMatch(token)
}

func OnMessage(ws *websocket.Conn, messages chan *matcher.Message) {
	defer ws.Close()
	for {
		msg := <-messages
		log.Printf("GOT PUBLISHED MESSAGE, sending to client: %s", msg)
		err := ws.WriteJSON(msg)
		if err != nil {
			log.Printf("Error sending message to client over websocket %s", err.Error())
		}
		if msg.Code == matcher.TERMINATE {
			break
		}
	}
}