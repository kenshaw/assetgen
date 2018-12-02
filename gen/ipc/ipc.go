package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// CallbackMap is a map of IPC callback handlers.
type CallbackMap map[string]func(...interface{}) (interface{}, error)

// Server handles IPC based callbacks for child processes.
type Server struct {
	sock string
	m    CallbackMap
	logf func(string, ...interface{})
}

// New creates a IPC callback server with the provided options and callback
// map.
func New(m CallbackMap, opts ...Option) (*Server, error) {
	sock, err := ioutil.TempDir("", "assetgen-ipc-callback")
	if err != nil {
		return nil, err
	}
	sock += "/control.sock"

	s := &Server{
		sock: sock,
		m:    m,
	}

	// apply opts
	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	if s.logf == nil {
		s.logf = log.Printf
	}

	return s, nil
}

// SocketPath returns the socket path for the server.
func (s *Server) SocketPath() string {
	return s.sock
}

// Run runs the server.
func (s *Server) Run(ctxt context.Context) error {
	ctxt, cancel := context.WithCancel(ctxt)

	l, err := net.Listen("unix", s.sock)
	if err != nil {
		return err
	}

	// sig handler
	go func() {
		defer cancel()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		s.logf("caught signal: %v", <-sig)
	}()

	go func() {
		defer l.Close()

		for {
			select {
			default:
				conn, err := l.Accept()
				if err != nil {
					s.logf("error: %v", err)
					return
				}
				go s.handle(ctxt, conn)

			case <-ctxt.Done():
				if err := ctxt.Err(); err != nil && err != context.Canceled {
					s.logf("error: %v", ctxt.Err())
				}
				return
			}
		}
	}()

	return nil
}

// handle handles incoming client connections.
func (s *Server) handle(ctxt context.Context, conn net.Conn) error {
	defer conn.Close()

	sn := bufio.NewScanner(conn)
	for {
		select {
		case <-ctxt.Done():
			return ctxt.Err()
		default:
			for sn.Scan() {
				// decode
				var v msg
				if err := json.NewDecoder(strings.NewReader(sn.Text())).Decode(&v); err != nil {
					s.logf("error decoding msg: %v", err)
					return err
				}

				// handle request
				ret := make(map[string]interface{}, 1)
				switch v.Type {
				case "list-functions":
					var funcs []string
					for fn := range s.m {
						funcs = append(funcs, fn)
					}
					ret["result"] = funcs

				case "call":
					res, err := s.doCall(v)
					if err != nil {
						ret["error"] = err.Error()
					} else {
						ret["result"] = res
					}

				default:
					ret["error"] = "unknown request type"
				}

				return json.NewEncoder(conn).Encode(ret)
			}
			if err := sn.Err(); err != nil && err != io.EOF {
				s.logf("error reading from socket: %v", err)
			}
		}
	}
}

// doCall passes calls to the callback map.
func (s *Server) doCall(v msg) (interface{}, error) {
	name, ok := v.Params["name"].(string)
	if !ok {
		return nil, errors.New("missing name in call")
	}
	args, ok := v.Params["args"].([]interface{})
	if !ok {
		return nil, errors.New("missing args in call")
	}
	f, ok := s.m[name]
	if !ok {
		return nil, errors.New("invalid func name")
	}
	return f(args...)
}

// msg is a simple envelope for messages passed between the executing
// javascript and the server.
type msg struct {
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params"`
}
