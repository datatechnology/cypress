package cypress

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

func testActions(t *testing.T) []Action {
	actions := []Action{
		Action{
			Name: "greeting",
			Handler: ActionHandler(func(r *http.Request, response *Response) {
				time.Sleep(time.Millisecond * 50)
				response.DoneWithContent(http.StatusAccepted, "text/html", []byte(fmt.Sprintf("<h1>Hello, %s</h1>", r.URL.String())))

				session, _ := r.Context().Value(SessionKey).(*Session)
				if session != nil {
					fmt.Println("SESSID:", session.ID)
				} else {
					t.Error("no session detected while one expected")
				}
			}),
		},
		Action{
			Name: "panic",
			Handler: ActionHandler(func(r *http.Request, response *Response) {
				panic("ask for panic")
			}),
		},
		Action{
			Name: "index",
			Handler: ActionHandler(func(request *http.Request, response *Response) {
				model := &TestModel{
					Title:   "Page Title",
					Message: "Page Content",
				}

				response.DoneWithTemplate(http.StatusOK, model, "index.tmpl", "header.tmpl")
			}),
		},
	}

	return actions[:]
}

func printSessionID(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		traceID := GetTraceID(request.Context())
		session := GetSession(request)
		fmt.Println(traceID, session.ID)
		handler.ServeHTTP(writer, request)
	})
}

func TestWebServer(t *testing.T) {
	SetupLogger(LogLevelDebug, os.Stdout)
	server := NewWebServer(":8099", NewTemplateManager("./test/tmpl", time.Second*10))
	defer server.Shutdown()

	server.AddUserProvider(&TestUserProvider{})
	server.WithSessionOptions(NewInMemorySessionStore(), 15*time.Minute)
	server.WithStandardRouting("/web")
	server.AddWsEndoint("/ws/echo", &TestWsListener{})
	server.RegisterController("test", ControllerFunc(func() []Action { return testActions(t) }))
	server.WithCustomHandler(CustomHandlerFunc(printSessionID))

	go func() {
		if err := server.Start(); err != nil {
			fmt.Println(err)
		}
	}()

	// wait for the server to start
	time.Sleep(time.Second)
	resp, err := http.Get("http://localhost:8099/web/test/greeting?ticket=test")
	if err != nil {
		t.Error("server is not started or working properly")
		return
	}

	if resp.StatusCode != http.StatusAccepted {
		t.Error("Unexpected http status")
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		t.Error("failed to read body")
		return
	}

	resp.Body.Close()
	fmt.Println(string(body))

	resp, err = http.Get("http://localhost:8099/web/test/index")
	if err != nil {
		t.Error("server is not started or working properly")
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Error("Unexpected http status")
		return
	}

	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Error("failed to read body")
		return
	}

	fmt.Println(string(body))

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
