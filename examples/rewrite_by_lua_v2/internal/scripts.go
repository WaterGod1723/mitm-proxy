package internal

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"text/template"
)

const chromeScriptTmpl = `#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
USER_DATA_DIR="$SCRIPT_DIR/chrome_dev_profile"
PROXY_ADDR="http://{{.Host}}:{{.Port}}"

echo "Starting Chrome with proxy: $PROXY_ADDR"
echo "User data dir: $USER_DATA_DIR"

mkdir -p "$USER_DATA_DIR"

OS="$(uname -s)"
case "$OS" in
	MINGW*|MSYS*|CYGWIN*)
		CHROME_PATH="/c/Program Files/Google/Chrome/Application/chrome.exe"
		if [ ! -f "$CHROME_PATH" ]; then
			CHROME_PATH="/c/Program Files (x86)/Google/Chrome/Application/chrome.exe"
		fi
		;;
	Darwin)
		CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		;;
	Linux)
		CHROME_PATH="/usr/bin/google-chrome"
		if [ ! -f "$CHROME_PATH" ]; then
			CHROME_PATH="/usr/bin/google-chrome-stable"
		fi
		;;
	*)
		CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		;;
esac

"$CHROME_PATH" \
  --user-data-dir="$USER_DATA_DIR" \
  --proxy-server="$PROXY_ADDR" \
  --ignore-certificate-errors \
  --no-first-run \
  --no-default-browser-check \
  --disable-web-security \
  --disable-features=SameSiteByDefaultCookies \
  --disable-site-isolation-trials \
  "$@"
`

type chromeScriptData struct {
	Host string
	Port string
}

// GenerateChromeScript 生成 Chrome 启动脚本。
func GenerateChromeScript(host, port, outPath string) error {
	tmpl, err := template.New("chrome").Parse(chromeScriptTmpl)
	if err != nil {
		return fmt.Errorf("parse chrome script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, chromeScriptData{Host: host, Port: port}); err != nil {
		return fmt.Errorf("execute chrome script template: %w", err)
	}

	if err := os.WriteFile(outPath, buf.Bytes(), 0755); err != nil {
		return fmt.Errorf("write chrome script: %w", err)
	}
	return nil
}

// GetLocalIP 获取本机第一个非回环 IPv4 地址。
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
