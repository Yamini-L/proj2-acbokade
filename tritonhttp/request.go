package tritonhttp

import (
	"fmt"
	"strings"
	"net"
	"io"
)

type Request struct {
	Method string // e.g. "GET"
	URL    string // e.g. "/path/to/a/file"
	Proto  string // e.g. "HTTP/1.1"

	// Headers stores the key-value HTTP headers
	Headers map[string]string

	Host  string // determine from the "Host" header
	Close bool   // determine from the "Connection" header
}

const (
	GET = "GET"
	POST = "POST"
	HOST = "Host"
	CONNECTION = "Connection"
	CLOSE = "close"
	responseProto = "HTTP/1.1"
	doubleCarriageReturnNewLine = "\r\n\r\n"
	carriageReturnNewLine = "\r\n"
)

// Method Split a full request into lines
func splitFullRequestIntoLines(fullRequest string) ([]string) {
	requestLines := make([] string, 0)
	for {
		index := strings.Index(fullRequest, carriageReturnNewLine)
		if index != -1 {
			fmt.Println("next line", fullRequest[:index])
			requestLines = append(requestLines, fullRequest[:index])
		}
		if index == -1 {
			requestLines = append(requestLines, fullRequest[:])
			break
		}
		fullRequest = fullRequest[index+2:]
		if fullRequest == carriageReturnNewLine {
			break
		}
	}
	return requestLines
}

func checkForFullRequestsInString(remaining *string) ([][]string) {
	// 2D array where first index correspond to request number in case of
	// pipelining and second index correspond to the line number of the request
	var linesArr[][] string
	for {
		if strings.Contains(*remaining, doubleCarriageReturnNewLine) {
			delimiterIndex := strings.Index(*remaining, doubleCarriageReturnNewLine)
			requestLines := splitFullRequestIntoLines((*remaining)[:delimiterIndex])
			linesArr = append(linesArr, requestLines)
			// Remove request read from remaining
			*remaining = (*remaining)[delimiterIndex+4:]
		} else {
			break
		}
	}
	return linesArr
}

func ReadAllLines(conn net.Conn, remaining *string) ([][]string, error) {
	// 2D array where first index correspond to request number in case of
	// pipelining and second index correspond to the line number of the request
	var linesArr [][]string 
	var err error = nil
	var requestsLinesArr [][]string
	var buf []byte = make([]byte, 1024)
	// Check if remaining contains a full request
	requestsLinesArr = checkForFullRequestsInString(remaining)
	if len(requestsLinesArr) > 0 {
		linesArr = append(linesArr, requestsLinesArr...)
	}
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			fmt.Println("EOF encountered")
		}
		fmt.Println("error before break", err)
		*remaining += string(buf[:n])
		return nil, err
	}
	*remaining += string(buf[:n])
	fmt.Println("Before remaining", *remaining, len(*remaining))
	requestsLinesArr = checkForFullRequestsInString(remaining)
	if len(requestsLinesArr) > 0 {
		linesArr = append(linesArr, requestsLinesArr...)
	}
	fmt.Println("After remaining", *remaining, len(*remaining))
	fmt.Println("LinesArr", linesArr)
	return linesArr, nil
}

// Method which reads all the requests (in case of pipelining) from a single client
func ReadAllRequests(conn net.Conn, remaining *string) ([][]string, error){
	allLines, err := ReadAllLines(conn, remaining)
	if err != nil {
		return nil, err
	}
	return allLines, nil
}

// Method which parses initial request line 
func parseRequestLine(line string) (string, string, string, error) {
	fields := strings.SplitN(line, " ", 3)
	fmt.Println("fields", fields, len(fields))
	if len(fields) != 3 {
		return "", "", "", fmt.Errorf("could not parse the request line")
	} 
	return fields[0], fields[1], fields[2], nil
}

// Method which reads request from the given reader of the connection
func HandleRequest(requestString []string) (req *Request, errors []error) {
	// Initialize request object
	req = &Request{}
	req.Headers = make(map[string]string)

	remainingLines := requestString[1:]
	var err error = nil
	for {
		if len(remainingLines) == 0 {
			break
		}
		line := remainingLines[0]
		res := strings.Split(line, ":")
		if len(res) != 2 {
			// Not in proper form: maybe colon is missing
			errors = append(errors, fmt.Errorf("error parsing request header"))
			remainingLines = remainingLines[1:]
			if len(remainingLines) == 0 {
				break
			}
			continue
		}
		key, value := res[0], res[1]
		key = CanonicalHeaderKey(key)
		// Remove all leading and trailing space from value
		value = strings.TrimSpace(value)
		fmt.Println("Key value", key, value)
		if key == HOST {
			fmt.Println("Setting host", value)
			req.Host = value
		} 
		if key == CONNECTION {
			fmt.Println("Setting Connection", value)
			req.Headers[CONNECTION] = value
		}
		remainingLines = remainingLines[1:]
		if len(remainingLines) == 0 {
			break
		}
	}

	// Read start line
	initialRequestLine := requestString[0]
	req.Method, req.URL, req.Proto, err = parseRequestLine(string(initialRequestLine))
	if err != nil {
		fmt.Println("Error while parsing request line", err)
		errors = append(errors, err)
		return req, errors
	}
	fmt.Println("Method: ", req.Method)
	// Only GET method is supported and well formed URL starts with /
	if req.Method != GET {
		fmt.Println("Invalid method")
		errors = append(errors, fmt.Errorf("invalid method"))
		return req, errors
	}
	if len(req.URL) == 0 || (req.URL[0] != '\\' && req.URL[0] != '/') {
		fmt.Println("URL doesnt start with slash")
		errors = append(errors, fmt.Errorf("url doesnt start with slash"))
		return req, errors
	}
	if req.Proto != responseProto {
		fmt.Println("protocol is not HTTP/1.1")
		errors = append(errors, fmt.Errorf("protocol is not HTTP/1.1"))
		return req, errors
	}
	return req, errors
}


