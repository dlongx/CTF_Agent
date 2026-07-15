package app

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"net/http/httptest"
	"testing"
)

func TestWebSocketAcceptKnownExample(t *testing.T) {
	t.Parallel()
	if got := websocketAccept("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("websocketAccept=%q", got)
	}
}

func TestWebSocketOriginPolicy(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "http://localhost/ws", nil)
	req.Host = "localhost:8000"
	req.Header.Set("Origin", "http://localhost:8000")
	if !websocketOriginAllowed(req, nil) {
		t.Fatal("same-origin websocket should be allowed")
	}
	req.Header.Set("Origin", "https://allowed.example")
	if !websocketOriginAllowed(req, []string{"https://allowed.example"}) {
		t.Fatal("configured origin should be allowed")
	}
	req.Header.Set("Origin", "https://blocked.example")
	if websocketOriginAllowed(req, []string{"https://allowed.example"}) {
		t.Fatal("unconfigured cross-origin websocket should be rejected")
	}
}

func TestWebSocketWritesExtendedTextFrame(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	connection := &wsConn{conn: server}
	payload := make([]byte, 300)
	for index := range payload {
		payload[index] = byte(index)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- connection.WriteText(string(payload)) }()
	reader := bufio.NewReader(client)
	first, _ := reader.ReadByte()
	second, _ := reader.ReadByte()
	if first != 0x81 || second != 126 {
		t.Fatalf("frame header=%x %x", first, second)
	}
	var length uint16
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil || length != 300 {
		t.Fatalf("frame length=%d err=%v", length, err)
	}
	got := make([]byte, length)
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatal("frame payload mismatch")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteText: %v", err)
	}
}

func TestWebSocketReadsMaskedCloseFrame(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	connection := &wsConn{conn: server, reader: bufio.NewReader(server)}
	errCh := make(chan error, 1)
	go func() { errCh <- connection.ReadUntilClose() }()
	mask := []byte{1, 2, 3, 4}
	payload := []byte{0x03, 0xE8}
	frame := []byte{0x88, 0x80 | byte(len(payload))}
	frame = append(frame, mask...)
	for index, value := range payload {
		frame = append(frame, value^mask[index%len(mask)])
	}
	if _, err := client.Write(frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ReadUntilClose: %v", err)
	}
}
