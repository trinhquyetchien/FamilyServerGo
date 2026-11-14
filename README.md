# FamilyServerGo - Ứng Dụng Hỗ Trợ Điều Khiển Điện Thoại Từ Xa

Ứng dụng Server & Desktop Native bằng Go (Golang) cho dự án hỗ trợ người thân điều khiển điện thoại từ xa qua mạng nội bộ hoặc Internet (ngrok domain cố định / Cloudflare Tunnel).

Ứng dụng thay thế hoàn toàn Node.js server cũ, tích hợp cả **WebSocket Relay Server hiệu năng cao** và **Giao diện Desktop Native Client** trong một file thực thi duy nhất (`family-remote`).

---

## 🌟 Tính Năng Nổi Bật

### 1. WebSocket Relay Server (Back-end Go)
- **Tương thích hoàn toàn với ứng dụng Android**: Lắng nghe WebSocket tại `/` và `/ws` trên cổng `8080`.
- **Hỗ trợ Domain Cố Định Ngrok**:
  - Tự động chạy tunnel ngrok với domain cố định: `relaxed-major-hen.ngrok-free.app`
  - URL truy cập: `https://relaxed-major-hen.ngrok-free.app` (WebSocket: `wss://relaxed-major-hen.ngrok-free.app/ws`).
  - Hỗ trợ truyền authtoken qua cờ `--authtoken=<token>` hoặc biến môi trường `NGROK_AUTHTOKEN`.
  - Fallback linh hoạt sang Cloudflare Tunnel hoặc địa chỉ mạng nội bộ.
- **Relay 2 chiều tức thì (Real-time Relay)**:
  - Nhận frame hình ảnh (JPEG/Base64 hoặc Binary bytes) từ điện thoại Android và phát tới tất cả Desktop/Web clients.
  - Tự động xoay màn hình (**Auto Screen Rotation**) theo tỷ lệ xoay Portrait (dọc) hoặc Landscape (ngang).
  - Gửi lệnh điều khiển chuột: `tap` (chạm), `swipe` (vuốt) với tọa độ chuẩn hóa `x, y` (0.0 đến 1.0).
  - Phím cứng hệ thống: `back`, `home`, `recents`, `notifications`, `power`, `lock`, `volume_up`, `volume_down`, `mute`.
  - Phím tắt mở nhanh cài đặt: `open_settings`, `open_app_permissions`, `open_accessibility`, `open_wifi`.
  - Lệnh dừng phiên từ xa: `disconnect`.
- **An toàn luồng (Thread-Safe & Race-Free)**: Quản lý kết nối an toàn với mutex trên từng socket (`safeConn`), vượt qua kiểm tra với cờ `-race`.
- **Hỗ trợ chạy Headless Server**: Cờ `--server-only` cho phép chạy dịch vụ nền trên VPS/Server mà không cần mở cửa sổ đồ họa.
- **Tự động cấu hình `adb reverse`**: Tự động chuyển tiếp cổng khi kết nối qua cáp USB.

### 2. Desktop Native GUI Viewer (Front-end Desktop Native Go)
- **Cửa sổ ứng dụng Native độc lập**: Khởi chạy dưới dạng cửa sổ ứng dụng desktop borderless chuyên biệt (`--app`), tích hợp trọn vẹn tài nguyên (HTML/JS/CSS/SVG) vào binary thông qua `go:embed`.
- **Hiển thị trực quan theo thời gian thực**: Render Canvas 60 FPS, mượt mà và có đo lường FPS.
- **Tự động thích ứng xoay màn hình**: Tự động chuyển đổi khung nhìn ngang/dọc tương ứng với thao tác xoay trên điện thoại.
- **Tương tác chuột thông minh**:
  - Click chuột trái: Tính toán tọa độ và thực hiện Tap.
  - Kéo và thả chuột: Tính toán góc, khoảng cách, tốc độ và thực hiện Swipe (vuốt màn hình).
- **Thanh Dock và Bảng điều khiển phím**: Đầy đủ các nút thao tác Home, Back, Recents, Phím nguồn, Âm lượng, Trợ năng, Cài đặt ứng dụng và Chụp màn hình lưu về máy tính.

---

## 📦 Cài Đặt & Biên Dịch

### Yêu Cầu Môi Trường
- Go 1.22 trở lên.
- Trình duyệt Chrome / Chromium hoặc `xdg-open` để hiển thị Native App Window.

### Các Lệnh Make

```bash
# Biên dịch file thực thi tối ưu
make build

# Chạy ứng dụng Desktop Native
make run

# Chạy ở chế độ Headless Server (chỉ chạy relay server cổng 8080)
make server

# Chạy toàn bộ Unit Test & Race Detector
make test

# Cài đặt lối tắt Desktop Shortcut và Icon trái tim lớn vào hệ thống Linux
make install

# Dọn dẹp
make clean
```

---

## 🖥️ Cấu Trúc Dự Án

```
FamilyServerGo/
├── Makefile                        # Tự động hóa build, test, install
├── README.md                       # Tài liệu hướng dẫn chi tiết
├── go.mod                          # Module Go
├── go.sum                          # Checksum dependencies
├── main.go                         # Core WebSocket Server & Native App Launcher
├── main_test.go                    # Unit & Integration Tests (100% race-free)
├── icon.svg                        # Icon Trái Tim Lớn (Squircle Deep Navy + Big Heart)
├── web/
│   └── index.html                  # Giao diện Native Viewer nhúng trực tiếp qua go:embed
└── linux/
    ├── desktop/
    │   └── family-remote.desktop   # File Desktop Entry chuẩn Linux XDG
    └── icons/
        ├── family-remote.svg       # Biểu tượng SVG chính thức
        └── heart-icon.svg          # Biểu tượng trái tim hỗ trợ gia đình
```

---

## 🚀 Hướng Dẫn Sử Dụng

1. **Khởi chạy ứng dụng trên máy tính**:
   ```bash
   ./family-remote
   ```
2. **Kết nối từ điện thoại Android**:
   - Nếu kết nối qua USB: Cắm cáp và ứng dụng sẽ tự động kích hoạt `adb reverse tcp:8080 tcp:8080`.
   - Nếu kết nối qua Wi-Fi nội bộ: Nhập địa chỉ `ws://<IP_MÁY_TÍNH>:8080/ws`.
   - Nếu kết nối qua Internet (4G/khác mạng): Quét mã QR hiển thị ở thanh bên trái ứng dụng hoặc nhập link `wss://...trycloudflare.com/ws`.
3. **Thao tác hỗ trợ**:
   - Điều khiển trực tiếp bằng chuột trên khung hình điện thoại.
   - Sử dụng bảng nút bên trái hoặc Dock nổi để điều hướng hệ thống.

---

## 📄 Bản Quyền
Dự án được phát triển riêng cho gia đình - Giữ bản quyền bởi **Trịnh Quyết Chiến** (https://github.com/trinhquyetchien).
