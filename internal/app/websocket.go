package app

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	websocketReadLimit    = 64 << 10
	websocketReadTimeout  = 75 * time.Second
	websocketWriteTimeout = 10 * time.Second
)

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
	if !websocketOriginAllowed(r, s.cfg.AllowedOrigins) {
		http.Error(w, "forbidden websocket origin", http.StatusForbidden)
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
	closed := make(chan struct{})
	go func() {
		_ = conn.ReadUntilClose()
		close(closed)
	}()
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case <-pingTicker.C:
			if err := conn.WriteControl(0x9, nil); err != nil {
				return
			}
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
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusBadRequest)
		return nil, errors.New("missing websocket upgrade")
	}
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		http.Error(w, "invalid websocket connection header", http.StatusBadRequest)
		return nil, errors.New("missing websocket connection upgrade token")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "unsupported websocket version", http.StatusUpgradeRequired)
		return nil, errors.New("unsupported websocket version")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	decodedKey, decodeErr := base64.StdEncoding.DecodeString(key)
	if decodeErr != nil || len(decodedKey) != 16 {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, errors.New("invalid websocket key")
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
	return &wsConn{conn: rawConn, reader: rw.Reader}, nil
}

func headerContainsToken(value string, token string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), token) {
			return true
		}
	}
	return false
}

func websocketOriginAllowed(r *http.Request, allowedOrigins []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(allowed), "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}

func websocketAccept(key string) string {
	hash := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (c *wsConn) Close() error {
	return c.conn.Close()
}

func (c *wsConn) WriteText(text string) error {
	return c.writeFrame(0x1, []byte(text))
}

func (c *wsConn) WriteControl(opcode byte, payload []byte) error {
	if len(payload) > 125 {
		return errors.New("websocket control payload too large")
	}
	return c.writeFrame(opcode, payload)
}

func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(websocketWriteTimeout))
	header := []byte{0x80 | opcode}
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
	if err := writeAll(c.conn, header); err != nil {
		return err
	}
	return writeAll(c.conn, payload)
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func (c *wsConn) ReadUntilClose() error {
	reader := c.reader
	if reader == nil {
		reader = bufio.NewReader(c.conn)
	}
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(websocketReadTimeout))
		first, err := reader.ReadByte()
		if err != nil {
			return err
		}
		second, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if first&0x80 == 0 {
			return errors.New("fragmented websocket frames are unsupported")
		}
		if second&0x80 == 0 {
			return errors.New("client websocket frame must be masked")
		}
		length := uint64(second & 0x7f)
		switch length {
		case 126:
			var encoded uint16
			if err := binary.Read(reader, binary.BigEndian, &encoded); err != nil {
				return err
			}
			length = uint64(encoded)
		case 127:
			if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
				return err
			}
		}
		if length > websocketReadLimit {
			return errors.New("websocket frame exceeds read limit")
		}
		var mask [4]byte
		if _, err := io.ReadFull(reader, mask[:]); err != nil {
			return err
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return err
		}
		for index := range payload {
			payload[index] ^= mask[index%len(mask)]
		}
		switch first & 0x0f {
		case 0x8:
			return nil
		case 0x9:
			if err := c.WriteControl(0xA, payload); err != nil {
				return err
			}
		case 0xA, 0x1, 0x2:
		default:
			return errors.New("unsupported websocket opcode")
		}
	}
}
