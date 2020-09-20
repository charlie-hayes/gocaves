package servers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
)

// HTTPRequest encapsulates an HTTP request.
type HTTPRequest struct {
	Method string
	URL    *url.URL
	Header http.Header
	Body   io.Reader
}

// PeekBody will return the full body and swap the reader with a
// new one which will allow other users to continue to use it.
func (r *HTTPRequest) PeekBody() []byte {
	data, _ := ioutil.ReadAll(r.Body)
	r.Body = bytes.NewReader(data)
	return data
}

// HTTPResponse encapsulates an HTTP response.
type HTTPResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.Reader
}

// PeekBody will return the full body and swap the reader with a
// new one which will allow other users to continue to use it.
func (r *HTTPResponse) PeekBody() []byte {
	data, _ := ioutil.ReadAll(r.Body)
	r.Body = bytes.NewReader(data)
	return data
}

// WithJSONBody sets the body of the response to a specific JSON value
// NOTE: This does not set headers or status code.
// NOTE: This returns a copy of the object.
func (r HTTPResponse) WithJSONBody(val interface{}) *HTTPResponse {
	data, _ := json.Marshal(val)
	r.Body = bytes.NewReader(data)
	return &r
}

// HTTPServerHandlers provides all the handlers for the http server
type HTTPServerHandlers struct {
	NewRequestHandler func(*HTTPRequest) *HTTPResponse
}

// HTTPServer is a generic implementation of an HTTP server used by
// the various HTTP servers in this mock.
type HTTPServer struct {
	serviceName string
	listenPort  int
	localAddr   string
	listener    net.Listener
	handlers    HTTPServerHandlers
	server      *http.Server
}

// NewHTTPServiceOptions enables the specification of default options for a new http server.
type NewHTTPServiceOptions struct {
	Name     string
	Handlers HTTPServerHandlers
}

// NewHTTPServer instantiates a new instance of the memd server.
func NewHTTPServer(opts NewHTTPServiceOptions) (*HTTPServer, error) {
	svc := &HTTPServer{
		serviceName: opts.Name,
		handlers:    opts.Handlers,
	}

	err := svc.start()
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// ServiceName returns the name of this service
func (s *HTTPServer) ServiceName() string {
	if s.serviceName == "" {
		return "Unknown Service"
	}
	return s.serviceName
}

// ListenPort returns the port this server is listening on.
func (s *HTTPServer) ListenPort() int {
	return s.listenPort
}

// Start will start this HTTP server
func (s *HTTPServer) start() error {
	// Generate a listen address, listenPort defaults to 0, which means by default
	// we will be using a random port, future attempts to start this same server
	// should however reuse the same port that we originally had used.
	listenAddr := fmt.Sprintf(":%d", s.listenPort)

	lsnr, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Printf("failed to start listening for http `%s` server: %s", s.ServiceName(), err)
		return err
	}

	addr := lsnr.Addr()
	tcpAddr := addr.(*net.TCPAddr)
	s.listenPort = tcpAddr.Port
	s.localAddr = addr.String()
	s.listener = lsnr

	srv := &http.Server{
		Handler: http.HandlerFunc(s.handleHTTP),
	}
	s.server = srv

	log.Printf("starting listener for %s (http) server on port %d", s.ServiceName(), s.listenPort)
	go func() {
		err := srv.Serve(s.listener)
		if err != nil {
			log.Printf("listener for http `%s` failed to serve: %s", s.ServiceName(), err)
		}
	}()

	return nil
}

// Close will stop this HTTP server
func (s *HTTPServer) Close() error {
	if s.server == nil {
		log.Printf("attempted to stop a stopped http `%s` server", s.ServiceName())
		return errors.New("cannot stop a stopped server")
	}

	err := s.server.Close()
	if err != nil {
		log.Printf("failed to stop listening for http `%s` server: %s", s.ServiceName(), err)
		return err
	}

	s.server = nil

	return nil
}

func (s *HTTPServer) handleHTTP(w http.ResponseWriter, req *http.Request) {
	resp := s.handlers.NewRequestHandler(&HTTPRequest{
		Method: req.Method,
		URL:    req.URL,
		Header: req.Header,
		Body:   req.Body,
	})

	if resp == nil {
		// If nobody decides to answer the request, we write 501 Unsupported.
		w.WriteHeader(501)
		return
	}

	for headerName, headerValues := range resp.Header {
		for _, headerValue := range headerValues {
			w.Header().Add(headerName, headerValue)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
