package cypress

import (
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

var (
	// ErrDupActionName mulitple actions are having the same name for the same controller
	ErrDupActionName = errors.New("action name duplicated")
)

// Action a named http.Handler
type Action struct {
	Name    string
	Handler http.Handler
}

// Controller a request controller that could provide a set of
// http.Handler to handle http requests based on the action name
type Controller interface {
	ListActions() []Action
}

// ControllerFunc a function that implements Controller interface
type ControllerFunc func() []Action

// ListActions adapt the ControllerFunc to Controller interface
func (f ControllerFunc) ListActions() []Action {
	return f()
}

// WebServer a web server that supports auth & authz, logging,
// session and web sockets
type WebServer struct {
	server             *http.Server
	router             *mux.Router
	securityHandler    *SecurityHandler
	sessionStore       SessionStore
	sessionTimeout     time.Duration
	registeredHandlers map[string]map[string]http.Handler
}

// NewWebServer creates a web server instance to listen on the
// specified address
func NewWebServer(listenAddr string) *WebServer {
	return &WebServer{
		server: &http.Server{
			Addr: listenAddr,
		},
		router:             mux.NewRouter(),
		securityHandler:    NewSecurityHandler(),
		sessionTimeout:     time.Minute * 30,
		registeredHandlers: make(map[string]map[string]http.Handler),
	}
}

// HandleFunc register a handle function for a path pattern
func (server *WebServer) HandleFunc(path string, f func(w http.ResponseWriter, r *http.Request)) *WebServer {
	server.router.HandleFunc(path, f)
	return server
}

// WithStandardRouting setup a routing as "prefix" + "/{controller:[_a-zA-Z][_a-zA-Z0-9]*}/{action:[_a-zA-Z][_a-zA-Z0-9]*}"
// and the web server will route the requests based on the registered controllers.
func (server *WebServer) WithStandardRouting(prefix string) *WebServer {
	server.router.HandleFunc(prefix+"/{controller:[_a-zA-Z][_a-zA-Z0-9]*}/{action:[_a-zA-Z][_a-zA-Z0-9]*}", server.routeRequest)
	return server
}

// RegisterController register a controller for the standard routing
func (server *WebServer) RegisterController(name string, controller Controller) error {
	actions, ok := server.registeredHandlers[name]
	if !ok {
		actions = make(map[string]http.Handler)
		server.registeredHandlers[name] = actions
	}

	for _, item := range controller.ListActions() {
		_, ok = actions[item.Name]
		if ok {
			return ErrDupActionName
		}

		actions[item.Name] = item.Handler
	}

	return nil
}

// AddUserProvider adds a user provider to security handler
func (server *WebServer) AddUserProvider(provider UserProvider) *WebServer {
	server.securityHandler.AddUserProvider(provider)
	return server
}

// WithAuthz specify the AuthorizationManager to be used by this handler
func (server *WebServer) WithAuthz(authz AuthorizationManager) *WebServer {
	server.securityHandler.WithAuthz(authz)
	return server
}

// AddWsEndoint adds a web socket endpoint to the server
func (server *WebServer) AddWsEndoint(endpoint string, listener WebSocketListener) *WebServer {
	wsHandler := &WebSocketHandler{
		Listener: listener,
	}
	server.router.HandleFunc(endpoint, wsHandler.Handle)
	return server
}

// AddStaticResource adds a static resource folder to the server with the given prefix,
// the prefix must be in format of "/prefix/"
func (server *WebServer) AddStaticResource(prefix, dir string) *WebServer {
	server.router.PathPrefix(prefix).Handler(http.StripPrefix(prefix, http.FileServer(http.Dir(dir))))
	return server
}

// WithSessionOptions setup the session options including the session store and session timeout interval
func (server *WebServer) WithSessionOptions(store SessionStore, timeout time.Duration) *WebServer {
	server.sessionStore = store
	server.sessionTimeout = timeout
	return server
}

// Shutdown shutdown the web server
func (server *WebServer) Shutdown() {
	server.server.Shutdown(nil)
}

// Start starts the web server
func (server *WebServer) Start() error {
	handler := http.Handler(server.securityHandler.WithPipeline(server.router))
	handler = NewSessionHandler(handler, server.sessionStore, server.sessionTimeout)
	handler = LoggingHandler(handler)
	handler = handlers.ProxyHeaders(handler)
	http.Handle("/", handler)
	return server.server.ListenAndServe()
}

func (server *WebServer) routeRequest(writer http.ResponseWriter, request *http.Request) {
	routeVars := mux.Vars(request)
	zap.L().Debug("routeRequest", zap.String("controller", routeVars["controller"]), zap.String("action", routeVars["action"]))
	if routeVars != nil {
		actions, ok := server.registeredHandlers[routeVars["controller"]]
		if ok {
			handler, ok := actions[routeVars["action"]]
			if ok {
				handler.ServeHTTP(writer, request)
				return
			}
		}
	}

	writer.WriteHeader(http.StatusNotFound)
	writer.Write([]byte("<h1>Not found</h1>"))
}
