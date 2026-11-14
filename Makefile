BINARY_NAME=family-remote
BUILD_DIR=.
DESKTOP_DIR=linux/desktop
ICONS_DIR=linux/icons

.PHONY: all build run server test clean install

all: build

build:
	@echo "🔨 Biên dịch ứng dụng $(BINARY_NAME)..."
	go build -ldflags="-s -w" -o $(BINARY_NAME) .
	@echo "✅ Biên dịch thành công: ./$(BINARY_NAME)"

run: build
	@echo "🚀 Khởi chạy Family Remote Desktop App..."
	./$(BINARY_NAME)

server: build
	@echo "🚀 Khởi chạy ở chế độ Headless Server (cổng 8080)..."
	./$(BINARY_NAME) --server-only --port 8080

test:
	@echo "🧪 Chạy toàn bộ kiểm thử Unit & Integration test..."
	go test -v -race ./...

clean:
	@echo "🧹 Dọn dẹp file biên dịch..."
	rm -f $(BINARY_NAME)

install: build
	@echo "📦 Đang cài đặt Desktop Entry và Icon hệ thống..."
	mkdir -p ~/.local/share/applications
	mkdir -p ~/.local/share/icons/hicolor/scalable/apps
	cp $(ICONS_DIR)/family-remote.svg ~/.local/share/icons/hicolor/scalable/apps/family-remote.svg
	cp $(DESKTOP_DIR)/family-remote.desktop ~/.local/share/applications/family-remote.desktop
	sed -i 's|Exec=.*|Exec=$(PWD)/$(BINARY_NAME)|' ~/.local/share/applications/family-remote.desktop
	sed -i 's|Icon=.*|Icon=$(PWD)/$(ICONS_DIR)/family-remote.svg|' ~/.local/share/applications/family-remote.desktop
	update-desktop-database ~/.local/share/applications 2>/dev/null || true
	@echo "✅ Đã cài đặt lối tắt Desktop thành công!"
