package tritonhttp

import (
	"fmt"
	"log"
	"os"
	"errors"
	"io"
	"net"
	"time"
)

const (
	TCP = "tcp"
)

type Server struct {
	// Addr specifies the TCP address for the server to listen on,
	// in the form "host:port". It shall be passed to net.Listen()
	// during ListenAndServe().
	Addr string // e.g. ":0"

	// VirtualHosts contains a mapping from host name to the docRoot path
	// (i.e. the path to the directory to serve static files from) for
	// all virtual hosts that this server supports
	VirtualHosts map[string]string
}

// Method which checks the validity of the current working directory
func (s Server) ValidateServerSetup() error {
	cwd, err := os.Getwd()
	fmt.Println("validity server setup cwd", cwd)
	if err != nil {
		log.Fatal(err)
	}
	fi, err := os.Stat(cwd)
	if os.IsNotExist(err) {
		return err
	}
	if !fi.IsDir() {
		return err
	}
	return nil
}

// ListenAndServe listens on the TCP network address s.Addr and then
// handles requests on incoming connections.
func (s *Server) ListenAndServe() error {
	if err := s.ValidateServerSetup(); err != nil {
		return err
	}
	// Create a listener
	listener, err := net.Listen(TCP, s.Addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.HandleConnection(conn)
	}
}

func (s *Server) HandleConnection(conn net.Conn) {
	// remaining is the string which contains the http request which is not
	// yet parsed but received
	var remaining string = ""
	for {
		fmt.Println("for loop")
		// Set both read and write timeout
		if err := conn.SetReadDeadline(time.Now().Add(RECV_TIMEOUT)); err != nil {
			_ = conn.Close()
			return
		}
		if err := conn.SetWriteDeadline(time.Now().Add(SEND_TIMEOUT)); err != nil {
			_ = conn.Close()
			return
		}

		// Read the requests sent by the client (it could be multiple HTTP requests)
		allLines, err := ReadAllRequests(conn, &remaining)
		// Handle each request sent
		for _, singleReq := range allLines {
			req, err := HandleRequest(singleReq)
			if err != nil {
				fmt.Println("first error", err)
				res := &Response{}
				res.Headers = make(map[string] string)
				res.HandleBadRequest()
				_ = res.Write(conn)
				_ = conn.Close()
				continue
			}

			// Handle errors
			// Error 1: Client has closed the conn => io.EOF error
			if errors.Is(err, io.EOF) {
				_ = conn.Close()
				continue
			}

			// Error 2: Timeout from the server --> net.Error
			// timeout in this application means we just close the connection
			// Note : proj3 might require you to do a bit more here
			if err, ok := err.(net.Error); ok && err.Timeout() {
				_ = conn.Close()
				continue
			}

			// Error 3: malformed/invalid request
			// Handle the request which is not a GET and immediately close the connection and return
			if err != nil {
				res := &Response{}
				res.Headers = make(map[string] string)
				res.HandleBadRequest()
				_ = res.Write(conn)
				_ = conn.Close()
				continue
			}

			// Host not present, send 400 client error
			if len(req.Host) == 0 {
				fmt.Println("host not present")
				res := &Response{}
				res.Headers = make(map[string] string)
				res.HandleBadRequest()
				_ = res.Write(conn)
				_ = conn.Close()
				continue
			}

			// Handle good request
			log.Println("Handling good request")
			res := s.HandleGoodRequest(req)
			err = res.Write(conn)
			if err != nil {
				fmt.Println(err)
			}
			if (req.Headers[CONNECTION] == CLOSE) {
				conn.Close()
				fmt.Println("handle connection returned")
				break
			}
		}
		// Connection timeout
		if (err != nil) {
			fmt.Println("ReadAllRequsts err", err)
			// Client hasn't send anything now, hence close the connection
			if (len(remaining) == 0) {
				_ = conn.Close()
				return
			}
			// Client has sent partial request
			// Respond with 400 client error
			res := &Response{}
			res.Headers = make(map[string] string)
			res.HandleBadRequest()
			_ = res.Write(conn)
			_ = conn.Close()
			return
		}

		// We'll never close the connection and handle as many requests for this connection and pass on this
		// responsibility to the timeout mechanism
	}
}
