package mongrel2

import (
	"fmt"
	"http"
	"testing"
)

func testHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("in handler")
	fmt.Fprintf(w, "hi")
}

func TestConn(t *testing.T) {
	Serve("82209006-86FF-4982-B5EA-D1E29E55D481", "tcp://127.0.0.1:9999", "tcp://127.0.0.1:9998", http.HandlerFunc(testHandler))
}
