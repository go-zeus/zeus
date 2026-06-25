package proxy

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// wsHandshake 发送原生 WebSocket 握手并返回建立的 TCP 连接
// 不依赖 gorilla/websocket，直接构造 HTTP Upgrade 请求
func wsHandshake(t *testing.T, addr, path string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}

	key := generateWSKey()
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		t.Fatalf("write handshake: %v", err)
	}

	// 读取握手响应（HTTP 状态行 + headers + \r\n）
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		conn.Close()
		t.Fatalf("expected 101 Switching Protocols, got %q", statusLine)
	}
	// 读完剩余 headers 直到空行
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			t.Fatalf("read header: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}
	return conn
}

// generateWSKey 生成 16 字节随机 base64 编码的 WS Key
func generateWSKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// wsWriteFrame 写入一个 WebSocket 文本帧（客户端→服务端必须 mask）
func wsWriteFrame(conn net.Conn, opcode byte, payload []byte) error {
	// FIN + opcode
	header := []byte{0x80 | opcode}
	// Mask + 长度
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	maskBit := byte(0x80)
	switch {
	case len(payload) < 126:
		header = append(header, maskBit|byte(len(payload)))
	case len(payload) < 65536:
		header = append(header, maskBit|126)
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(payload)))
		header = append(header, l[:]...)
	default:
		header = append(header, maskBit|127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(len(payload)))
		header = append(header, l[:]...)
	}
	header = append(header, mask...)
	// 掩码 payload
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	if _, err := conn.Write(append(header, masked...)); err != nil {
		return err
	}
	return nil
}

// wsReadFrame 读取一个 WebSocket 帧（服务端→客户端无 mask）
func wsReadFrame(conn net.Conn) (byte, []byte, error) {
	br := bufio.NewReader(conn)
	h := make([]byte, 2)
	if _, err := io.ReadFull(br, h); err != nil {
		return 0, nil, err
	}
	opcode := h[0] & 0x0f
	length := int(h[1] & 0x7f)
	switch length {
	case 126:
		var l [2]byte
		if _, err := io.ReadFull(br, l[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(l[:]))
	case 127:
		var l [8]byte
		if _, err := io.ReadFull(br, l[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint64(l[:]))
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return 0, nil, err
	}
	return opcode, payload, nil
}

// computeAccept 计算 WebSocket Accept 值用于响应校验
func computeAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// TestWebSocket_EchoRoundtrip 验证 WebSocket 握手 + 消息往返
//
// 后端实现一个简易 echo WS 服务：
//  1. 验证 Upgrade 握手（计算 Sec-WebSocket-Accept）
//  2. 收到消息后 echo 返回
//
// 代理应当 raw 透传，两端均应感知到握手成功
func TestWebSocket_EchoRoundtrip(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 WS 握手
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "not websocket", http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()

		// 写握手响应
		key := r.Header.Get("Sec-WebSocket-Key")
		accept := computeAccept(key)
		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
		_, _ = bufrw.WriteString(resp)
		_ = bufrw.Flush()

		// echo 循环：读一帧、写回
		for {
			opcode, payload, err := wsReadFrameFromHijack(bufrw)
			if err != nil {
				return
			}
			if opcode == 0x8 { // close
				return
			}
			// 写回 echo（不 mask）
			frame := buildUnmaskedFrame(opcode, payload)
			_, _ = bufrw.Write(frame)
			_ = bufrw.Flush()
		}
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	// 通过代理做 WS 握手
	proxyAddr := strings.TrimPrefix(proxySrv.URL, "http://")
	conn := wsHandshake(t, proxyAddr, "/ws")
	defer conn.Close()

	// 发送一条文本消息
	msg := []byte("hello zeus ws proxy")
	if err := wsWriteFrame(conn, 0x1, msg); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	// 读取 echo
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	opcode, payload, err := wsReadFrame(conn)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if opcode != 0x1 {
		t.Errorf("opcode = %d, want 1 (text)", opcode)
	}
	if string(payload) != string(msg) {
		t.Errorf("payload = %q, want %q", string(payload), string(msg))
	}
}

// wsReadFrameFromHijack 从 hijacked bufio 读取 WS 帧（用于后端）
func wsReadFrameFromHijack(br *bufio.ReadWriter) (byte, []byte, error) {
	h := make([]byte, 2)
	if _, err := io.ReadFull(br, h); err != nil {
		return 0, nil, err
	}
	opcode := h[0] & 0x0f
	masked := h[1]&0x80 != 0
	length := int(h[1] & 0x7f)
	switch length {
	case 126:
		var l [2]byte
		if _, err := io.ReadFull(br, l[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(l[:]))
	case 127:
		var l [8]byte
		if _, err := io.ReadFull(br, l[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint64(l[:]))
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}

// buildUnmaskedFrame 构造无 mask 的 WS 帧（服务端→客户端）
func buildUnmaskedFrame(opcode byte, payload []byte) []byte {
	header := []byte{0x80 | opcode}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) < 65536:
		header = append(header, 126)
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(payload)))
		header = append(header, l[:]...)
	default:
		header = append(header, 127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(len(payload)))
		header = append(header, l[:]...)
	}
	return append(header, payload...)
}

// TestWebSocket_BackendDown_Returns502 验证后端不可用时返回 502
func TestWebSocket_BackendDown_Returns502(t *testing.T) {
	// 后端地址不可用：使用保留端口
	target, _ := url.Parse("http://127.0.0.1:1")
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	proxyAddr := strings.TrimPrefix(proxySrv.URL, "http://")
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	// 发送 WS 握手到不可用后端
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + proxyAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = conn.Write([]byte(req))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(statusLine, "502") {
		t.Errorf("status = %q, want contains 502", statusLine)
	}
}
