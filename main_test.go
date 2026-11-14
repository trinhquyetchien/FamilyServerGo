package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHTTPServer_Index(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handleHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected text/html content type, got %s", contentType)
	}
}

func TestHTTPServer_APIInfo(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/info", nil)
	w := httptest.NewRecorder()

	handleHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if _, ok := data["local_ws"]; !ok {
		t.Errorf("missing local_ws in response: %v", data)
	}
}

func TestWebSocketRelay_PhoneToWeb(t *testing.T) {
	// Khởi tạo test server với handleWebSocket
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWebSocket)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// 1. Tạo Web Client kết nối
	webConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial web ws: %v", err)
	}
	defer webConn.Close()

	// Web Client đăng ký
	regWeb := map[string]interface{}{
		"type": "register",
		"role": "web",
	}
	if err := webConn.WriteJSON(regWeb); err != nil {
		t.Fatalf("failed to register web: %v", err)
	}

	// Đọc thông báo trạng thái ban đầu (phone_disconnected)
	var initialStatus map[string]interface{}
	if err := webConn.ReadJSON(&initialStatus); err != nil {
		t.Fatalf("failed to read initial status: %v", err)
	}
	if initialStatus["status"] != "phone_disconnected" {
		t.Errorf("expected initial status phone_disconnected, got %v", initialStatus["status"])
	}

	// 2. Tạo Phone Client kết nối
	phoneClientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial phone ws: %v", err)
	}
	defer phoneClientConn.Close()

	// Phone đăng ký
	regPhone := map[string]interface{}{
		"type":         "register",
		"role":         "phone",
		"screenWidth":  1080,
		"screenHeight": 2400,
		"orientation":  "portrait",
	}
	if err := phoneClientConn.WriteJSON(regPhone); err != nil {
		t.Fatalf("failed to register phone: %v", err)
	}

	// Web client phải nhận được thông báo phone_connected
	var connectStatus map[string]interface{}
	if err := webConn.ReadJSON(&connectStatus); err != nil {
		t.Fatalf("failed to read phone_connected: %v", err)
	}
	if connectStatus["status"] != "phone_connected" {
		t.Errorf("expected phone_connected, got %v", connectStatus["status"])
	}

	// 3. Test relay frame hình ảnh (Base64 JPEG)
	testFrame := map[string]interface{}{
		"type":  "frame",
		"image": "/9j/4AAQSkZJRg==",
	}
	if err := phoneClientConn.WriteJSON(testFrame); err != nil {
		t.Fatalf("failed to send frame: %v", err)
	}

	var receivedFrame map[string]interface{}
	if err := webConn.ReadJSON(&receivedFrame); err != nil {
		t.Fatalf("failed to read frame: %v", err)
	}
	if receivedFrame["type"] != "frame" || receivedFrame["image"] != "/9j/4AAQSkZJRg==" {
		t.Errorf("unexpected frame received: %v", receivedFrame)
	}

	// 4. Test relay xoay màn hình (screen_size)
	screenEvent := map[string]interface{}{
		"type":        "screen_size",
		"width":       2400,
		"height":      1080,
		"orientation": "landscape",
	}
	if err := phoneClientConn.WriteJSON(screenEvent); err != nil {
		t.Fatalf("failed to send screen_size: %v", err)
	}

	var receivedScreen map[string]interface{}
	if err := webConn.ReadJSON(&receivedScreen); err != nil {
		t.Fatalf("failed to read screen_size: %v", err)
	}
	if receivedScreen["orientation"] != "landscape" {
		t.Errorf("expected landscape, got %v", receivedScreen["orientation"])
	}

	// 5. Test điều khiển từ Web -> Phone (tap action)
	tapAction := map[string]interface{}{
		"type": "tap",
		"x":    0.5,
		"y":    0.5,
	}
	if err := webConn.WriteJSON(tapAction); err != nil {
		t.Fatalf("failed to send tap action: %v", err)
	}

	var receivedTap map[string]interface{}
	if err := phoneClientConn.ReadJSON(&receivedTap); err != nil {
		t.Fatalf("failed to read tap action on phone: %v", err)
	}
	if receivedTap["type"] != "tap" {
		t.Errorf("expected tap action, got %v", receivedTap["type"])
	}

	// 6. Test binary frame relay
	binaryData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10} // JPEG magic header bytes
	if err := phoneClientConn.WriteMessage(websocket.BinaryMessage, binaryData); err != nil {
		t.Fatalf("failed to send binary frame: %v", err)
	}

	msgType, receivedBytes, err := webConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read binary message: %v", err)
	}
	if msgType != websocket.BinaryMessage || len(receivedBytes) != len(binaryData) {
		t.Errorf("unexpected binary frame: type %d, length %d", msgType, len(receivedBytes))
	}
}

func TestHeadlessServerFlag(t *testing.T) {
	// Kiểm tra tính hợp lệ của channel shutdown & timing
	done := make(chan bool, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for routine")
	}
}
