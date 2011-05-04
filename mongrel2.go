package mongrel2

import (
	"bytes"
	"fmt"
	"http"
	"io/ioutil"
	"json"
	"os"
	"strconv"
	"github.com/edsrzf/zegomq"
)

func Serve(identity, pullAddr, pubAddr string, handler http.Handler) os.Error {
	pull, err := zmq.NewSocket(zmq.SOCK_PULL, "")
	if err != nil {
		return err
	}
	pull.Bind("tcp", pullAddr)
	pub, err := zmq.NewSocket(zmq.SOCK_PUB, identity)
	if err != nil {
		return err
	}
	pub.Bind("tcp", pubAddr)

	b := []byte(nil)
	for {
		b, err = pull.ReadFrame(b)
		if err != nil {
			panic(err.String())
		}
		split := bytes.Split(b, []byte{' '}, 4)
		if len(split) < 4 {
			panic("bad parse")
		}
		uuid, id, path := split[0], split[1], split[2]
		headerJson, n := parseNetstring(split[3])
		fmt.Printf("whole request: %q\n", split[3])
		if split[3][n] != ',' {
			// error
		}
		n++
		var header map[string]string
		err := json.Unmarshal(headerJson, &header)
		if err != nil {
			panic(err.String())
		}

		body, nn := parseNetstring(split[3][n:])
		n += nn
		if split[3][n] != ',' {
			// error
		}

		fmt.Printf("uuid: %q\nid: %q\npath: %q\nheader: %v\nbody: %q\n", uuid, id, path, header, body)
		req, err := makeRequest(header)
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		resp := response{buf: bytes.NewBuffer(nil), header: http.Header{}}
		handler.ServeHTTP(resp, req)
		fmt.Printf("response:\n%q\n", resp.buf.Bytes())
		fmt.Fprintf(pub, "%s %s, %s", uuid, netstring(id), resp.buf.Bytes())
	}
	panic("unreachable")
}

var skipHeader = map[string]bool{
	"PATH":           true,
	"METHOD":         true,
	"VERSION":        true,
	"URI":            true,
	"PATTERN":        true,
	"Host":           true,
	"Referer":        true,
	"User-Agent":     true,
	"Content-Length": true,
}

func makeRequest(params map[string]string) (*http.Request, os.Error) {
	r := new(http.Request)
	r.Method = params["METHOD"]
	if r.Method == "" {
		return nil, os.NewError("mongrel2: no METHOD")
	}

	r.Proto = params["VERSION"]
	var ok bool
	r.ProtoMajor, r.ProtoMinor, ok = http.ParseHTTPVersion(r.Proto)
	if !ok {
		return nil, os.NewError("mongrel2: invalid protocol version")
	}

	r.Trailer = http.Header{}
	r.Header = http.Header{}

	r.Host = params["Host"]
	r.Referer = params["Referer"]
	r.UserAgent = params["User-Agent"]

	if lenstr := params["Content-Length"]; lenstr != "" {
		clen, err := strconv.Atoi64(lenstr)
		if err != nil {
			return nil, os.NewError("mongrel2: bad Content-Length")
		}
		r.ContentLength = clen
	}

	for k, v := range params {
		if !skipHeader[k] {
			r.Header.Add(k, v)
		}
	}

	// TODO: cookies

	if r.Host != "" {
		r.RawURL = "http://" + r.Host + params["URI"]
		url, err := http.ParseURL(r.RawURL)
		if err != nil {
			return nil, os.NewError("mongrel2: failed to parse host and URI into a URL")
		}
		r.URL = url
	}
	if r.URL == nil {
		r.RawURL = params["URI"]
		url, err := http.ParseURL(r.RawURL)
		if err != nil {
			return nil, os.NewError("mongrel2: failed to parse URI into a URL")
		}
		r.URL = url
	}

	// TODO: how do we know if we're using HTTPS?
	// TODO: fill in r.RemoteAddr

	return r, nil
}

type response struct {
	buf         *bytes.Buffer
	wroteHeader bool
	header      http.Header
}

func (r response) Header() http.Header { return r.header }

func (r response) Write(b []byte) (int, os.Error) {
	r.WriteHeader(http.StatusOK)
	return r.buf.Write(b)
}

func (r response) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true

	fmt.Fprintf(r.buf, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	r.header.Write(r.buf)
	r.buf.WriteString("\r\n")
}

func parseNetstring(nstr []byte) ([]byte, int) {
	i := bytes.IndexByte(nstr, ':')
	if i < 0 {
		panic("not a netstring?")
	}
	n, err := strconv.Atoi(string(nstr[:i]))
	if err != nil {
		panic("invalid number before colon")
	}
	if n > len(nstr[i+1:]) {
		panic("netstring length too long")
	}
	count := i + 1 + n
	return nstr[i+1 : count], count
}

func netstring(str []byte) []byte {
	l := strconv.Itoa(len(str))
	b := make([]byte, len(l)+1+len(str))
	i := copy(b, l)
	b[i] = ':'
	copy(b[i+1:], str)
	return b
}