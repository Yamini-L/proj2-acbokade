package tritonhttp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Response struct {
	Proto      string // e.g. "HTTP/1.1"
	StatusCode int    // e.g. 200
	StatusText string // e.g. "OK"

	// Headers stores all headers to write to the response.
	Headers map[string]string

	// Request is the valid request that leads to this response.
	// It could be nil for responses not resulting from a valid request.
	// Hint: you might need this to handle the "Connection: Close" requirement
	Request *Request

	// FilePath is the local path to the file to serve.
	// It could be "", which means there is no file to serve.
	FilePath string
}

const (
	statusOK = http.StatusOK
	statusBadRequest = http.StatusBadRequest
	statusNotFound = http.StatusNotFound
)

var statusText = map[int]string{
	statusOK: "OK",
	statusBadRequest: "Bad Request",
	statusNotFound: "Not Found",
}


// HandleOK prepares res to be a 200 OK response
// ready to be written back to client.
func (res *Response) HandleOK() {
	res.StatusCode = statusOK
}

// HandleBadRequest prepares res to be a 405 Method Not allowed response
func (res *Response) HandleStatusNotFound() {
	res.AddProto(responseProto)
	res.StatusCode = statusNotFound
	res.FilePath = ""
	res.Headers = make(map[string]string)
	res.Headers["Connection"] = "close"
	res.Headers["Date"] = FormatTime(time.Now())
}

// HandleBadRequest prepares res to be a 405 Method Not allowed response
func (res *Response) HandleBadRequest() {
	res.AddProto(responseProto)
	res.StatusCode = statusBadRequest
	res.FilePath = ""
	res.Headers = make(map[string]string)
	res.Headers["Connection"] = "close"
	res.Headers["Date"] = FormatTime(time.Now())
}

func (res *Response) AddProto(proto string) {
	res.Proto = proto
}

func (s *Server) HandleGoodRequest(req *Request) (res *Response) {
	res = &Response{}
	res.Headers = make(map[string] string)
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	projectDir := filepath.Join(cwd, "../../")
	host := req.Host
	url := req.URL
	connection := req.Headers[CONNECTION]
	virtualHost, exists := s.VirtualHosts[host]
	fmt.Println("exists", exists)
	fmt.Println("url ", url)
	fmt.Println("host ", host)
	if !exists {
		fmt.Println("host not exists in virtualHost")
		res.HandleBadRequest()
		return res
	}
	reqFile := filepath.Join(cwd, virtualHost)
	fmt.Println("reqFile", reqFile,cwd,virtualHost)
	fmt.Println("url", url)
	// If URL ends with /, interpret as index.html
	if url[len(url) - 1] == '/' {
		fmt.Println("url ends with /")
		reqFile = filepath.Join(reqFile, url, "index.html")
	} else {
		reqFile = filepath.Join(reqFile, url)
	}
	fmt.Println("before clean", reqFile)
	reqFile = filepath.Clean(reqFile)
	fmt.Println("after clean", reqFile)
	// Check if reqFile is outside the parent directory
	// cwd must be a substring of reqFile
	if !strings.HasPrefix(reqFile, projectDir) {
		res.HandleStatusNotFound()
		return res
	}
	res.FilePath = reqFile
	fmt.Println("reqFile", reqFile)
	// Read file
	res.AddProto(responseProto)
	stats, err := os.Stat(reqFile)
	if errors.Is(err, os.ErrNotExist) {
		log.Println("No file", err)
		res.HandleStatusNotFound()
		return res
	}
	res.StatusCode = 200 
	fmt.Println("stats", stats)
	res.Headers["Content-Length"] = strconv.FormatInt(stats.Size(), 10)
	res.Headers["Content-Type"] = MIMETypeByExtension(filepath.Ext(reqFile))
	res.Headers["Date"] = FormatTime(time.Now())
	res.Headers["Last-Modified"] = FormatTime(stats.ModTime())
	if connection == CLOSE {
		res.Headers["Connection"] = CLOSE
	}
	fmt.Println("Response to be sent", res)
	return res
}

func (res *Response) Write(w io.Writer) error {
	bw := bufio.NewWriter(w)
	// Write status line
	statusLine := fmt.Sprintf("%v %v %v\r\n", res.Proto, res.StatusCode, statusText[res.StatusCode])
	fmt.Println("write statusLine:",statusLine)
	if _, err := bw.WriteString(statusLine); err != nil {
		return err
	}
	// Write Headers
	headers := res.Headers
	headerKeys := make([] string, 0)
	for key, _ := range headers {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)

	for _, key := range headerKeys {
		keyValue := key + ": " + headers[key] + "\r\n"
		if _, err := bw.WriteString(keyValue); err != nil {
			return err
		}
		fmt.Println("write header:",keyValue)
	}
	if _, err := bw.WriteString("\r\n"); err != nil {
		return err
	}

	// Write Body
	filePath := res.FilePath
	if len(filePath) > 0 {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if _, err := bw.Write(data); err != nil {
			return err
		}
		fmt.Println("write data:",data)
	}
	if err := bw.Flush(); err != nil {
		return nil
	}

	return nil
}