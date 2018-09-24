package cypress

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/websocket"

	"github.com/gorilla/mux"
)

type TestUserProvider struct{}

func (p *TestUserProvider) GetName() string {
	return "testProvider"
}

func (p *TestUserProvider) Authenticate(r *http.Request) *UserPrincipal {
	ticket := r.URL.Query().Get("ticket")
	if ticket != "" {
		return &UserPrincipal{
			ID:     ticket,
			Name:   ticket,
			Domain: "test",
			Roles:  make([]string, 0),
		}
	}

	return nil
}

func (p *TestUserProvider) Load(domain, id string) *UserPrincipal {
	return &UserPrincipal{
		ID:     id,
		Domain: domain,
		Name:   id,
		Roles:  make([]string, 0),
	}
}

type TestWsListener struct{}

func (l *TestWsListener) OnConnect(session *WebSocketSession) {
	fmt.Println("a websocket with session id", session.Session.ID, "is connected")
}

func (l *TestWsListener) OnClose(session *WebSocketSession, reason int) {
	fmt.Println("a websocket with session id", session.Session.ID, "has closed with reason", reason)
}

func (l *TestWsListener) OnTextMessage(session *WebSocketSession, message string) {
	fmt.Println("receive a text message", message)
	err := session.SendTextMessage(message)
	if err != nil {
		fmt.Println("failed to send message due to error", err)
	}
}

func (l *TestWsListener) OnBinaryMessage(session *WebSocketSession, message []byte) {
	fmt.Println("receive a binary message")
	err := session.SendBinaryMessage(message)
	if err != nil {
		fmt.Println("failed to send message due to error", err)
	}
}

func TestWebServer(t *testing.T) {
	s := &http.Server{
		Addr: ":8099",
	}
	defer s.Shutdown(nil)

	SetupLogger(LogLevelDebug, os.Stdout)

	securityHandler := NewSecurityHandler()
	securityHandler.AddUserProvider(&TestUserProvider{})

	wsHandler := &WebSocketHandler{
		Listener: &TestWsListener{},
	}

	router := mux.NewRouter()
	router.HandleFunc("/greeting", func(wr http.ResponseWriter, r *http.Request) {
		wr.WriteHeader(http.StatusAccepted)
		time.Sleep(time.Millisecond * 50)
		fmt.Fprintf(wr, "<h1>hello, %s</h1>", r.URL.String())

		session, _ := r.Context().Value(SessionKey).(*Session)
		if session != nil {
			fmt.Println("SESSID:", session.ID)
		} else {
			t.Error("no session detected while one expected")
		}
	})
	router.HandleFunc("/panic", func(wr http.ResponseWriter, r *http.Request) {
		panic("ask for panic")
	})

	router.HandleFunc("/ws/echo", wsHandler.Handle)

	handler := http.Handler(securityHandler.WithPipeline(router))
	handler = NewSessionHandler(handler, NewInMemorySessionStore(), 15*time.Minute)
	handler = LoggingHandler(handler)
	handler = handlers.ProxyHeaders(handler)
	http.Handle("/", handler)
	go func() {
		if err := s.ListenAndServe(); err != nil {
			fmt.Println(err)
		}
	}()

	// wait for the server to start
	time.Sleep(time.Second)
	resp, err := http.Get("http://localhost:8099/greeting?ticket=test")
	if err != nil {
		t.Error("server is not started or working properly")
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Error("failed to read body")
		return
	}

	fmt.Print(string(body))

	// try websocket
	c, _, err := websocket.DefaultDialer.Dial("ws://localhost:8099/ws/echo", nil)
	if err != nil {
		t.Error("dial:", err)
		return
	}

	defer c.Close()
	c.WriteMessage(websocket.TextMessage, []byte("Hello, websocket!"))
	msgType, msg, err := c.ReadMessage()
	if msgType != websocket.TextMessage || err != nil || string(msg) != "Hello, websocket!" {
		t.Error("failed to read back the message")
	}
}
