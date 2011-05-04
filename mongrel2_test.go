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
	Serve("4321", "127.0.0.1:9999", "127.0.0.1:9998", http.HandlerFunc(testHandler))
}
