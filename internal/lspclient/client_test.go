package lspclient

import (
	"bufio"
	"strconv"
	"strings"
	"testing"
)

// This is intentionally a disposable framing contract test, not a transcript
// golden. A future transport replacement should delete it freely.
func TestReadContentLengthFrame(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`
	client := &Client{reader: bufio.NewReader(strings.NewReader(
		"Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n" + payload,
	))}
	message, err := client.read()
	if err != nil {
		t.Fatal(err)
	}
	if !sameID(message.ID, 7) || string(message.Result) != `{"ok":true}` {
		t.Fatalf("message = %#v", message)
	}
}
