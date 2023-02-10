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

func (res *Response) AddProto(proto string) {
	res.Proto = proto
}

func (res *Response) HandleStatusNotFound() {
	res.AddProto(responseProto)
	res.StatusCode = statusNotFound
	res.FilePath = ""
	res.Headers = make(map[string]string)
	res.Headers["Date"] = FormatTime(time.Now())
}

func (res *Response) HandleBadRequest() {
	res.AddProto(responseProto)
	res.StatusCode = statusBadRequest
	res.FilePath = ""
	res.Headers = make(map[string]string)
	res.Headers["Date"] = FormatTime(time.Now())
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
	fmt.Println("Exists: ", exists)
	fmt.Println("Url: ", url)
	fmt.Println("Host: ", host)
	fmt.Println("VirtualHost: ", virtualHost)
	fmt.Println("Cwd: ", cwd)
	if !exists {
		fmt.Println("Host not exists in virtualHost")
		res.HandleBadRequest()
		if (req.Headers[CONNECTION] == CLOSE) {
			res.Headers[CONNECTION] = CLOSE
		}
		return res
	}
	reqFile := filepath.Join(cwd, virtualHost)
	// If URL ends with /, interpret as index.html
	if url[len(url) - 1] == '/' {
		fmt.Println("Url ends with /")
		reqFile = filepath.Join(reqFile, url, "index.html")
	} else {
		reqFile = filepath.Join(reqFile, url)
	}
	fmt.Println("Before clean: ", reqFile)
	reqFile = filepath.Clean(reqFile)
	fmt.Println("After clean: ", reqFile)
	// Check if reqFile is outside the parent directory
	// cwd must be a substring of reqFile
	if !strings.HasPrefix(reqFile, projectDir) {
		res.HandleStatusNotFound()
		return res
	}
	res.FilePath = reqFile
	fmt.Println("ReqFile: ", reqFile)
	// Read file
	res.AddProto(responseProto)
	stats, err := os.Stat(reqFile)
	if errors.Is(err, os.ErrNotExist) {
		log.Println("No file", err)
		res.HandleStatusNotFound()
		return res
	}
	res.StatusCode = 200 
	fmt.Println("Stats: ", stats)
	res.Headers["Content-Length"] = strconv.FormatInt(stats.Size(), 10)
	res.Headers["Content-Type"] = MIMETypeByExtension(filepath.Ext(reqFile))
	res.Headers["Date"] = FormatTime(time.Now())
	res.Headers["Last-Modified"] = FormatTime(stats.ModTime())
	if connection == CLOSE {
		res.Headers["Connection"] = CLOSE
	}
	// fmt.Println("Response to be sent: ", res)
	return res
}

func (res *Response) Write(w io.Writer) error {
	bw := bufio.NewWriter(w)
	// Write status line
	statusLine := fmt.Sprintf("%v %v %v\r\n", res.Proto, res.StatusCode, statusText[res.StatusCode])
	fmt.Println("Write statusLine: ",statusLine)
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
		fmt.Println("Write header: ",keyValue)
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
		fmt.Println("Write data:", len(data))
	}
	if err := bw.Flush(); err != nil {
		return nil
	}
	return nil
}