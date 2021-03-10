package gen

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

// IpcCallbackMap is a map of IPC callback handlers.
type IpcCallbackMap map[string]func(...interface{}) (interface{}, error)

// IpcServer handles IPC based callbacks for child processes.
type IpcServer struct {
	sock string
	m    IpcCallbackMap
	logf func(string, ...interface{})
}

// NewIpcServer creates a IPC server with the provided options and callback
// map. Handles simple IPC calls for "list-functions" and "call" that will
// provide the child process the ability to speak to the parent process.
func NewIpcServer(m IpcCallbackMap, opts ...IpcServerOption) (*IpcServer, error) {
	sock, err := ioutil.TempDir("", "assetgen-ipc-callback")
	if err != nil {
		return nil, err
	}
	sock += "/control.sock"
	s := &IpcServer{
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
func (s *IpcServer) SocketPath() string {
	return s.sock
}

// Run runs the server.
func (s *IpcServer) Run(ctxt context.Context) error {
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
		s.logf("caught signal: %s", <-sig)
	}()
	go func() {
		defer l.Close()
		for {
			select {
			default:
				conn, err := l.Accept()
				if err != nil {
					s.logf("error: %w", err)
					return
				}
				go s.handle(ctxt, conn)
			case <-ctxt.Done():
				if err := ctxt.Err(); err != nil && err != context.Canceled {
					s.logf("error: %w", ctxt.Err())
				}
				return
			}
		}
	}()
	return nil
}

// handle handles incoming client connections.
func (s *IpcServer) handle(ctxt context.Context, conn net.Conn) error {
	defer conn.Close()
	sn := bufio.NewScanner(conn)
	for {
		select {
		case <-ctxt.Done():
			return ctxt.Err()
		default:
			for sn.Scan() {
				// decode
				var v IpcMsg
				if err := json.NewDecoder(strings.NewReader(sn.Text())).Decode(&v); err != nil {
					s.logf("error decoding msg: %w", err)
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
				s.logf("error reading from socket: %w", err)
			}
		}
	}
}

// doCall passes calls to the callback map.
func (s *IpcServer) doCall(v IpcMsg) (interface{}, error) {
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

// IpcMsg is a simple envelope for messages passed between the executing
// javascript and the server.
type IpcMsg struct {
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params"`
}

// IpcServerOption is a IPC server option.
type IpcServerOption func(*IpcServer) error
