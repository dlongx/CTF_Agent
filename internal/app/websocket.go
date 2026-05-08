package app

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func (s *Service) websocketHandler(w http.ResponseWriter, r *http.Request) {
	taskID, err := taskIDFromWebSocketPath(r.URL.Path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, ok := s.store.Get(taskID); !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	logs, _ := s.store.Logs(taskID)
	if tail := r.URL.Query().Get("tail"); tail != "" {
		if limit, err := strconv.Atoi(tail); err == nil && limit > 0 && len(logs) > limit {
			logs = logs[len(logs)-limit:]
		}
	}
	if logs != "" {
		_ = conn.WriteText(logs)
	}
	sub := s.hub.Subscribe(taskID)
	defer s.hub.Unsubscribe(taskID, sub)

	for {
		select {
		case <-r.Context().Done():
			return
		case text, ok := <-sub:
			if !ok {
				return
			}
			if err := conn.WriteText(text); err != nil {
				return
			}
		}
	}
}

type wsConn struct {
	conn net.Conn
	mu   sync.Mutex
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusBadRequest)
		return nil, errors.New("missing websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, errors.New("missing websocket key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unsupported", http.StatusInternalServerError)
		return nil, errors.New("hijack unsupported")
	}
	rawConn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}
	accept := websocketAccept(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := rw.WriteString(response); err != nil {
		rawConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		rawConn.Close()
		return nil, err
	}
	return &wsConn{conn: rawConn}, nil
}

func websocketAccept(key string) string {
	hash := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (c *wsConn) Close() error {
	return c.conn.Close()
}

func (c *wsConn) WriteText(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	payload := []byte(text)
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[2:4], uint16(len(payload)))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[2:10], uint64(len(payload)))
	}
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}
