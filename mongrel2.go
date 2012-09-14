package mongrel2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/edsrzf/zegomq"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func Serve(identity, pullAddr, pubAddr string, handler http.Handler) error {
	c := zmq.NewContext()
	pull, err := c.NewSocket(zmq.SOCK_PULL, "")
	if err != nil {
		return err
	}
	if err = pull.Connect(pullAddr); err != nil {
		return err
	}
	pub, err := c.NewSocket(zmq.SOCK_PUB, identity)
	if err != nil {
		return err
	}
	if err = pub.Connect(pubAddr); err != nil {
		return err
	}

	for {
		msg, err := pull.RecvMsg()
		if err != nil {
			panic(err.Error())
		}
		b, err := ioutil.ReadAll(msg)
		if err != nil {
			panic(err.Error())
		}
		msg.Close()
		split := bytes.SplitN(b, []byte{' '}, 4)
		if len(split) < 4 {
			panic("bad parse")
		}
		uuid, id := split[0], split[1]
		headerJson, n := parseNetstring(split[3])
		var header map[string]string
		if err = json.Unmarshal(headerJson, &header); err != nil {
			panic(err.Error())
		}

		body, _ := parseNetstring(split[3][n:])

		req, err := makeRequest(header)
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		resp := response{buf: bytes.NewBuffer(nil), header: http.Header{}}
		handler.ServeHTTP(&resp, req)
		_, err = fmt.Fprintf(pub, "%s %s, %s", uuid, netstring(id), resp.buf.Bytes())
		if err != nil {
			panic(err.Error())
		}
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

func makeRequest(params map[string]string) (*http.Request, error) {
	r := new(http.Request)
	r.Method = params["METHOD"]
	if r.Method == "" {
		return nil, errors.New("mongrel2: no METHOD")
	}

	r.Proto = params["VERSION"]
	var ok bool
	r.ProtoMajor, r.ProtoMinor, ok = http.ParseHTTPVersion(r.Proto)
	if !ok {
		return nil, errors.New("mongrel2: invalid protocol version")
	}

	r.Trailer = http.Header{}
	r.Header = http.Header{}

	r.Host = params["Host"]
	r.Header.Set("Referer", params["Referer"])
	r.Header.Set("User-Agent", params["User-Agent"])

	if lenstr := params["Content-Length"]; lenstr != "" {
		clen, err := strconv.ParseInt(lenstr, 10, 64)
		if err != nil {
			return nil, errors.New("mongrel2: bad Content-Length")
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
		url_, err := url.Parse("http://" + r.Host + params["URI"])
		if err != nil {
			return nil, errors.New("mongrel2: failed to parse host and URI into a URL")
		}
		r.URL = url_
	}
	if r.URL == nil {
		url_, err := url.Parse(params["URI"])
		if err != nil {
			return nil, errors.New("mongrel2: failed to parse URI into a URL")
		}
		r.URL = url_
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

// get header
func (r response) Header() http.Header {
	return r.header
}

// write headers and body
func (r response) Write(b []byte) (int, error) {
	r.header.Set("Content-Length", strconv.Itoa(len(b)))
	r.WriteHeader(http.StatusOK) // does nothing if headers have been set before

	r.header.Write(r.buf)        // write headers
	r.buf.WriteString("\r\n")    // delimiter between headers and body
	return r.buf.Write(b)        // write body
}

// set headers, except content-length. does not write the headers.
func (r *response) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true

	if r.header.Get("Content-Type") == "" {
		r.header.Set("Content-Type", "text/html; charset=utf-8")
	}

	if r.header.Get("Date") == "" {
		r.header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}

	fmt.Fprintf(r.buf, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
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
	if nstr[count] != ',' {
		panic("netstring doesn't end with a comma")
	}
	return nstr[i+1 : count], count + 1
}

func netstring(str []byte) []byte {
	l := strconv.Itoa(len(str))
	b := make([]byte, len(l)+1+len(str))
	i := copy(b, l)
	b[i] = ':'
	copy(b[i+1:], str)
	return b
}
