package core_refactor

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// localIPs 返回本机非回环 IPv4 地址集合，用于检测请求环回。
func localIPs() (map[string]struct{}, error) {
	ips := make(map[string]struct{})
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				ips[ipNet.IP.String()] = struct{}{}
			}
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no valid non-loopback IPv4 address found")
	}
	ips["localhost"] = struct{}{}
	ips["127.0.0.1"] = struct{}{}
	return ips, nil
}

// isWebSocketRequest 判断请求是否为 WebSocket 升级请求。
func isWebSocketRequest(r *http.Request) bool {
	conn := r.Header.Get("Connection")
	upgrade := r.Header.Get("Upgrade")
	if !strings.Contains(strings.ToLower(conn), "upgrade") || !strings.EqualFold(upgrade, "websocket") {
		return false
	}
	return r.Header.Get("Sec-WebSocket-Key") != "" && r.Header.Get("Sec-WebSocket-Version") != ""
}

// hostPort 拆分 host 与端口；未指定端口时根据 isTLS 推断默认值。
func hostPort(host string, isTLS bool) (string, string) {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		h = host
		if isTLS {
			p = "443"
		} else {
			p = "80"
		}
	}
	return h, p
}

// targetPort 返回请求目标端口；优先使用 URL/Host 中的显式端口，否则按协议推断。
func targetPort(req *http.Request, clientTLS bool) string {
	_, port := hostPort(req.Host, clientTLS)
	if req.URL.Scheme == "https" {
		port = "443"
	} else if req.URL.Scheme == "http" {
		port = "80"
	}
	if _, p, err := net.SplitHostPort(req.Host); err == nil {
		port = p
	}
	return port
}
