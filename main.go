package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed web/index.html
var indexHTML []byte

type safeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (s *safeConn) WriteMessage(messageType int, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteMessage(messageType, data)
}

func (s *safeConn) WriteJSON(v interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(v)
}

func (s *safeConn) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Close()
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  10 * 1024 * 1024, // 10MB cho mỗi message frame
		WriteBufferSize: 10 * 1024 * 1024,
	}

	phoneMu   sync.Mutex
	phoneConn *safeConn
	phoneInfo map[string]interface{}

	webMu      sync.Mutex
	webClients = make(map[*safeConn]bool)

	fixedDomain = "relaxed-major-hen.ngrok-free.app"
	tunnelURL   = "https://" + fixedDomain
	tunnelMu    sync.RWMutex

	// Thống kê frame
	statsMu     sync.Mutex
	frameCount  int
	lastLogTime = time.Now()
)

func setTunnelURL(url string) {
	tunnelMu.Lock()
	tunnelURL = url
	tunnelMu.Unlock()
}

func getTunnelURL() string {
	tunnelMu.RLock()
	defer tunnelMu.RUnlock()
	return tunnelURL
}

func broadcastToWeb(messageType int, message []byte) {
	webMu.Lock()
	clients := make([]*safeConn, 0, len(webClients))
	for client := range webClients {
		clients = append(clients, client)
	}
	webMu.Unlock()

	for _, client := range clients {
		if err := client.WriteMessage(messageType, message); err != nil {
			log.Printf("[Relay] Lỗi gửi tới Web viewer: %v", err)
			client.Close()
			webMu.Lock()
			delete(webClients, client)
			webMu.Unlock()
		}
	}
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") == "websocket" {
		handleWebSocket(w, r)
		return
	}
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(indexHTML)
	case "/api/info":
		w.Header().Set("Content-Type", "application/json")
		phoneMu.Lock()
		hasPhone := (phoneConn != nil)
		phoneMu.Unlock()
		info := map[string]interface{}{
			"tunnel_url": getTunnelURL(),
			"local_ws":   "/ws",
			"has_phone":  hasPhone,
		}
		json.NewEncoder(w).Encode(info)
	default:
		http.NotFound(w, r)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS Upgrade Error]: %v", err)
		return
	}
	sConn := &safeConn{conn: conn}
	defer sConn.Close()

	for {
		msgType, msg, err := sConn.conn.ReadMessage()
		if err != nil {
			break
		}

		// Xử lý Binary message (JPEG frame trực tiếp)
		if msgType == websocket.BinaryMessage {
			statsMu.Lock()
			frameCount++
			statsMu.Unlock()
			broadcastToWeb(websocket.BinaryMessage, msg)
			continue
		}

		// Xử lý Text message (JSON)
		var payload map[string]interface{}
		if err := json.Unmarshal(msg, &payload); err != nil {
			continue
		}

		payloadType, _ := payload["type"].(string)

		// 1. Điện thoại Android đăng ký vai trò
		if payloadType == "register" && payload["role"] == "phone" {
			phoneMu.Lock()
			if phoneConn != nil && phoneConn != sConn {
				log.Println("[Register] Đã có phiên điện thoại cũ, thay thế bằng phiên mới")
				phoneConn.Close()
			}
			phoneConn = sConn
			phoneInfo = payload
			phoneMu.Unlock()

			log.Printf("--> [Phone Connected] Điện thoại bố mẹ đã sẵn sàng! (%v x %v, %v)",
				payload["screenWidth"], payload["screenHeight"], payload["orientation"])

			statusMsg, _ := json.Marshal(map[string]interface{}{
				"type":         "status",
				"status":       "phone_connected",
				"screenWidth":  payload["screenWidth"],
				"screenHeight": payload["screenHeight"],
				"orientation":  payload["orientation"],
			})
			broadcastToWeb(websocket.TextMessage, statusMsg)
			continue
		}

		// 2. Web Controller / Desktop Native viewer đăng ký
		if payloadType == "register" && payload["role"] == "web" {
			webMu.Lock()
			webClients[sConn] = true
			totalWeb := len(webClients)
			webMu.Unlock()

			log.Printf("--> [Web Connected] Client mới kết nối! Tổng số viewer: %d", totalWeb)

			phoneMu.Lock()
			hasPhone := (phoneConn != nil)
			pInfo := phoneInfo
			phoneMu.Unlock()

			if hasPhone {
				statusMsg, _ := json.Marshal(map[string]interface{}{
					"type":         "status",
					"status":       "phone_connected",
					"screenWidth":  pInfo["screenWidth"],
					"screenHeight": pInfo["screenHeight"],
					"orientation":  pInfo["orientation"],
				})
				sConn.WriteMessage(websocket.TextMessage, statusMsg)
			} else {
				statusMsg, _ := json.Marshal(map[string]interface{}{
					"type":   "status",
					"status": "phone_disconnected",
				})
				sConn.WriteMessage(websocket.TextMessage, statusMsg)
			}
			continue
		}

		// 3. Sự kiện kích thước và tự động xoay màn hình (Auto Screen Rotation)
		if payloadType == "screen_size" {
			phoneMu.Lock()
			if phoneInfo != nil {
				phoneInfo["screenWidth"] = payload["width"]
				phoneInfo["screenHeight"] = payload["height"]
				phoneInfo["orientation"] = payload["orientation"]
			}
			phoneMu.Unlock()
			log.Printf("--> [Rotate Event] Màn hình điện thoại xoay: %v x %v (%v)",
				payload["width"], payload["height"], payload["orientation"])
			broadcastToWeb(websocket.TextMessage, msg)
			continue
		}

		// 4. Frame hình ảnh truyền từ điện thoại qua JSON (Base64 JPEG)
		if payloadType == "frame" {
			statsMu.Lock()
			frameCount++
			statsMu.Unlock()
			broadcastToWeb(websocket.TextMessage, msg)
			continue
		}

		// 5. Lệnh điều khiển gửi từ Desktop Native Client tới Điện thoại
		switch payloadType {
		case "tap", "swipe", "back", "home", "recents", "notifications",
			"power", "lock", "volume_up", "volume_down", "mute",
			"open_settings", "open_app_permissions", "open_accessibility", "open_wifi",
			"disconnect":
			phoneMu.Lock()
			targetPhone := phoneConn
			phoneMu.Unlock()
			if targetPhone != nil {
				if err := targetPhone.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Printf("[Command Send Error]: %v", err)
				}
			} else {
				log.Printf("[Command Ignored] Bỏ qua lệnh '%s' vì điện thoại chưa kết nối", payloadType)
			}
		}
	}

	// Dọn dẹp kết nối khi thoát
	phoneMu.Lock()
	if sConn == phoneConn {
		phoneConn = nil
		phoneInfo = nil
		log.Println("[Phone Disconnected] Điện thoại bố mẹ đã ngắt kết nối")
		statusMsg, _ := json.Marshal(map[string]interface{}{
			"type":   "status",
			"status": "phone_disconnected",
		})
		broadcastToWeb(websocket.TextMessage, statusMsg)
	}
	phoneMu.Unlock()

	webMu.Lock()
	delete(webClients, sConn)
	webMu.Unlock()
}

func startFPSLogger() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			statsMu.Lock()
			now := time.Now()
			elapsed := now.Sub(lastLogTime).Seconds()
			if frameCount > 0 && elapsed > 0 {
				fps := float64(frameCount) / elapsed
				webMu.Lock()
				numWeb := len(webClients)
				webMu.Unlock()
				log.Printf("[Stream Status] Đang truyền %.1f FPS tới %d viewer(s)", fps, numWeb)
				frameCount = 0
				lastLogTime = now
			}
			statsMu.Unlock()
		}
	}()
}

func startNgrokTunnel(domain string, port int, authtoken string) *exec.Cmd {
	cmdPath, err := exec.LookPath("ngrok")
	if err != nil {
		homeDir, _ := os.UserHomeDir()
		localBin := homeDir + "/.local/bin/ngrok"
		if _, err := os.Stat(localBin); err == nil {
			cmdPath = localBin
		} else {
			log.Println("[ngrok] Không tìm thấy lệnh 'ngrok', bỏ qua tunnel ngrok")
			return nil
		}
	}

	if authtoken != "" {
		_ = exec.Command(cmdPath, "config", "add-authtoken", authtoken).Run()
	}

	args := []string{"http", fmt.Sprintf("--url=%s", domain), fmt.Sprintf("%d", port)}
	log.Printf("[ngrok] Khởi chạy ngrok: %s %v", cmdPath, args)
	cmd := exec.Command(cmdPath, args...)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[ngrok Pipe Error]: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[ngrok Start Error]: %v (sẽ fallback Cloudflare tunnel)", err)
		return nil
	}

	// Đọc log để phát hiện lỗi xác thực hoặc kết nối thành công
	if stderrPipe != nil {
		go func() {
			buf := make([]byte, 2048)
			for {
				n, err := stderrPipe.Read(buf)
				if n > 0 {
					out := string(buf[:n])
					if strings.Contains(out, "ERR_NGROK_") {
						log.Printf("[ngrok Error]: %s", strings.TrimSpace(out))
						break
					}
				}
				if err != nil {
					break
				}
			}
		}()
	}

	return cmd
}

func startCloudflaredTunnel(port int) *exec.Cmd {
	cmdPath, err := exec.LookPath("cloudflared")
	if err != nil {
		homeDir, _ := os.UserHomeDir()
		localBin := homeDir + "/.local/bin/cloudflared"
		if _, err := os.Stat(localBin); err == nil {
			cmdPath = localBin
		} else {
			log.Println("[Tunnel] Không tìm thấy lệnh 'cloudflared', bỏ qua tunnel ngầm")
			return nil
		}
	}

	log.Printf("[Tunnel] Đang khởi chạy Cloudflare Tunnel cho cổng %d...", port)
	cmd := exec.Command(cmdPath, "tunnel", "--url", fmt.Sprintf("http://localhost:%d", port))

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[Tunnel Pipe Error]: %v", err)
		return nil
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[Tunnel Start Error]: %v", err)
		return nil
	}

	go func() {
		buf := make([]byte, 2048)
		re := regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				match := re.FindString(output)
				if match != "" {
					setTunnelURL(match)
					log.Printf("=======================================================")
					log.Printf("🎉 CLOUDFLARE TUNNEL URL: %s", match)
					log.Printf("WebSocket URL cho điện thoại: %s/ws", regexp.MustCompile(`^https://`).ReplaceAllString(match, "wss://"))
					log.Printf("=======================================================")
					break
				}
			}
			if err != nil {
				break
			}
		}
	}()

	return cmd
}

func launchAppWindow(targetURL string) *exec.Cmd {
	browsers := []string{"google-chrome", "google-chrome-stable", "chromium-browser", "chromium", "brave"}
	for _, b := range browsers {
		if path, err := exec.LookPath(b); err == nil {
			args := []string{
				fmt.Sprintf("--app=%s", targetURL),
				"--window-size=1200,900",
				"--class=family-remote",
				"--user-data-dir=" + os.TempDir() + "/family_remote_profile",
			}
			cmd := exec.Command(path, args...)
			if err := cmd.Start(); err == nil {
				log.Printf("[Native Window] Đã mở Desktop App Window qua %s (--app)", b)
				return cmd
			}
		}
	}

	// Fallback mở bằng xdg-open nếu không có Chrome/Chromium
	if xdg, err := exec.LookPath("xdg-open"); err == nil {
		cmd := exec.Command(xdg, targetURL)
		cmd.Start()
		return cmd
	}

	return nil
}

func main() {
	serverOnly := flag.Bool("server-only", false, "Chạy chế độ Headless WebSocket Relay Server (không mở giao diện Desktop GUI)")
	portFlag := flag.Int("port", 8080, "Cổng lắng nghe của server")
	domainFlag := flag.String("domain", fixedDomain, "Domain ngrok cố định")
	authtokenFlag := flag.String("authtoken", os.Getenv("NGROK_AUTHTOKEN"), "ngrok authtoken (nếu cần thiết lập)")
	useCloudflare := flag.Bool("cloudflare", false, "Ép buộc sử dụng Cloudflare tunnel ngầm thay vì ngrok")
	flag.Parse()

	port := *portFlag
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		if !*serverOnly {
			log.Printf("Cổng %d đang bận, tự động chuyển cổng khác: %v", port, err)
			l, err = net.Listen("tcp", ":0")
			if err != nil {
				log.Fatalf("Không thể mở cổng server: %v", err)
			}
			port = l.Addr().(*net.TCPAddr).Port
		} else {
			log.Fatalf("Cổng %d đang bận: %v", port, err)
		}
	}
	l.Close()

	// Thiết lập domain cố định
	if *domainFlag != "" {
		setTunnelURL("https://" + *domainFlag)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHTTP)
	mux.HandleFunc("/ws", handleWebSocket)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		log.Printf("=======================================================")
		log.Printf("🚀 Family Remote Go Server đang chạy tại cổng :%d", port)
		log.Printf("🌐 Web Viewer URL: http://localhost:%d", port)
		log.Printf("📱 Android WebSocket URL: ws://<IP>:%d/ws", port)
		if *domainFlag != "" {
			log.Printf("🔒 Domain cố định: https://%s", *domainFlag)
			log.Printf("📱 Android WS (Internet): wss://%s/ws", *domainFlag)
		}
		log.Printf("=======================================================")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Lỗi server HTTP/WebSocket: %v", err)
		}
	}()

	startFPSLogger()

	// Thiết lập adb reverse nếu có thiết bị kết nối USB
	go func() {
		if adbPath, err := exec.LookPath("adb"); err == nil {
			exec.Command(adbPath, "reverse", fmt.Sprintf("tcp:%d", port), fmt.Sprintf("tcp:%d", port)).Run()
		}
	}()

	// Khởi chạy tunnel: Ưu tiên ngrok với domain cố định, fallback Cloudflare
	var tunnelCmd *exec.Cmd
	if !*useCloudflare && *domainFlag != "" {
		tunnelCmd = startNgrokTunnel(*domainFlag, port, *authtokenFlag)
	}
	// Nếu không có ngrok hoặc chỉ định cloudflare -> khởi chạy Cloudflare
	if tunnelCmd == nil {
		tunnelCmd = startCloudflaredTunnel(port)
	}

	var windowCmd *exec.Cmd
	if !*serverOnly {
		// Mặc định mở cửa sổ với domain cố định nếu có, hoặc fallback local
		targetURL := getTunnelURL()
		if targetURL == "" {
			targetURL = fmt.Sprintf("http://127.0.0.1:%d", port)
		}
		log.Printf("[Native Window] Đang mở cửa sổ ứng dụng: %s", targetURL)
		windowCmd = launchAppWindow(targetURL)
	} else {
		log.Println("[Headless Mode] Server đang chạy chế độ không mở cửa sổ GUI Desktop.")
	}

	// Lắng nghe tín hiệu tắt OS
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	done := make(chan bool)
	if windowCmd != nil {
		go func() {
			windowCmd.Wait()
			done <- true
		}()
	}

	select {
	case <-sigChan:
		log.Println("\n[Exit] Nhận tín hiệu dừng từ hệ điều hành...")
	case <-done:
		log.Println("\n[Exit] Cửa sổ ứng dụng Native Desktop đã được đóng...")
	}

	// Dọn dẹp tài nguyên
	if tunnelCmd != nil && tunnelCmd.Process != nil {
		tunnelCmd.Process.Kill()
	}
	server.Close()
	log.Println("[Exit] Đã dọn dẹp và tắt hoàn tất Family Remote Server.")
}
