package tritonhttp

import (
	// "fmt"
	"log"
	"os"
	"net"
	"time"
)

const (
	TCP = "tcp"
	RECIEVE_TIMEOUT time.Duration = 5 * time.Second
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
	log.Println("Validity server setup cwd: ", cwd)
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
	// Remaining is the string which contains the http request which is not
	// yet parsed but received
	var remaining string = ""
	for {
		log.Println("## For loop #")
		// Set both read and write timeout
		if err := conn.SetReadDeadline(time.Now().Add(RECIEVE_TIMEOUT)); err != nil {
			_ = conn.Close()
			return
		}
		// if err := conn.SetWriteDeadline(time.Now().Add(SEND_TIMEOUT)); err != nil {
		// 	_ = conn.Close()
		// 	return
		// }

		// Read the requests sent by the client (it could be multiple HTTP requests)
		allLines, err := ReadAllRequests(conn, &remaining)
		// Handle each request sent
		for _, singleReq := range allLines {
			req, errors := HandleRequest(singleReq)
			if len(errors) > 0 {
				log.Println("******** Handle Request Error **********")
				log.Println("Errors: ", errors)
				res := &Response{}
				res.Headers = make(map[string] string)
				res.HandleBadRequest()
				if (req.Headers[CONNECTION] == CLOSE) {
					res.Headers[CONNECTION] = CLOSE
				}
				_ = res.Write(conn)
				if (req.Headers[CONNECTION] == CLOSE) {
					_ = conn.Close()
					return
				}
				continue
			}

			// Host not present, send 400 client error
			if len(req.Host) == 0 {
				log.Println("Host not present")
				res := &Response{}
				res.Headers = make(map[string] string)
				res.HandleBadRequest()
				if (req.Headers[CONNECTION] == CLOSE) {
					res.Headers[CONNECTION] = CLOSE
				}
				_ = res.Write(conn)
				if (req.Headers[CONNECTION] == CLOSE) {
					_ = conn.Close()
					return
				}
				continue
			}

			// Handle good request
			log.Println("Handling good request")
			res := s.HandleGoodRequest(req)
			err = res.Write(conn)
			if err != nil {
				log.Println("Res Write: ", err)
			}
			if (req.Headers[CONNECTION] == CLOSE) {
				conn.Close()
				log.Println("Handle connection returned")
				break
			}
		}
	
		// Connection timeout
		if (err != nil) {
			log.Println("********* Connection timeout **********")
			log.Println("ReadAllRequests err: ", err)
			log.Println("Length of remaining: ", len(remaining))
			// log.Println("Is error timeout: ", err.(net.Error).Timeout())
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
			err = res.Write(conn)
			if err != nil {
				log.Println("Res Write: ", err)
			}
			_ = conn.Close()
			return
		}

		// We'll never close the connection and handle as many requests for this connection and pass on this
		// responsibility to the timeout mechanism
	}
}
