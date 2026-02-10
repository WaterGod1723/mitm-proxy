package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestRouteRule 测试路由规则功能
func TestRouteRule(t *testing.T) {
	// 创建测试代理实例
	proxy := NewDevToolProxy()

	// 定义测试用例
	testCases := []struct {
		name           string
		host           string
		path           string
		expectedHost   string
		expectedTarget string
		routeRules     []RouteRule
	}{
		{
			name:         "Basic Route Rule",
			host:         "httpbin.org",
			path:         "/test/path",
			expectedHost: "httpbin.org:443",
			routeRules: []RouteRule{
				{
					ID:              "route_1",
					Name:            "Test Route",
					MatchHost:       "httpbin.org",
					MatchPathPrefix: "/test",
					TargetHost:      "httpbin.org",
					TargetPort:      443,
					Enabled:         true,
				},
			},
		},
		{
			name:         "API Route Rule",
			host:         "api.example.com",
			path:         "/v1/users",
			expectedHost: "api-dev.example.com:443",
			routeRules: []RouteRule{
				{
					ID:              "route_2",
					Name:            "API Route",
					MatchHost:       "api.example.com",
					MatchPathPrefix: "/v1",
					TargetHost:      "api-dev.example.com",
					TargetPort:      443,
					Enabled:         true,
				},
			},
		},
		{
			name:         "No Matching Route",
			host:         "nonexistent.com",
			path:         "/random/path",
			expectedHost: "nonexistent.com", // Should remain unchanged
			routeRules: []RouteRule{
				{
					ID:              "route_3",
					Name:            "Test Route",
					MatchHost:       "httpbin.org",
					MatchPathPrefix: "/test",
					TargetHost:      "httpbin.org",
					TargetPort:      443,
					Enabled:         true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 设置测试配置
			proxy.config.Routes = tc.routeRules
			proxy.config.Rewrites = []RewriteRule{} // 清空重写规则

			// 创建测试请求
			req := httptest.NewRequest("GET", fmt.Sprintf("http://%s%s", tc.host, tc.path), nil)
			req.Host = tc.host

			// 应用路由规则
			proxy.applyRouting(req)

			// 检查结果
			if req.Host != tc.expectedHost {
				t.Errorf("Expected host %s, got %s", tc.expectedHost, req.Host)
			}

			expectedURLHost := fmt.Sprintf("%s:%d", tc.routeRules[0].TargetHost, tc.routeRules[0].TargetPort)
			if tc.routeRules[0].Enabled && strings.HasPrefix(tc.path, tc.routeRules[0].MatchPathPrefix) && tc.host == tc.routeRules[0].MatchHost {
				if req.URL.Host != expectedURLHost {
					t.Errorf("Expected URL host %s, got %s", expectedURLHost, req.URL.Host)
				}
			} else {
				// 如果没有匹配的路由规则，主机应该保持不变
				if req.URL.Host != tc.expectedHost {
					t.Errorf("Expected URL host %s, got %s", tc.expectedHost, req.URL.Host)
				}
			}
		})
	}
}

// TestRewriteRule 测试路径重写规则功能
func TestRewriteRule(t *testing.T) {
	// 创建测试代理实例
	proxy := NewDevToolProxy()

	// 定义测试用例
	testCases := []struct {
		name            string
		path            string
		expectedPath    string
		rewriteRules    []RewriteRule
		expectedNewHost string // 如果重写规则设置了新的主机
	}{
		{
			name:         "Basic Path Rewrite",
			path:         "/api/test/users",
			expectedPath: "/api/v1/users",
			rewriteRules: []RewriteRule{
				{
					ID:          "rewrite_1",
					Name:        "Test Rewrite",
					MatchPath:   "/api/test/(.*)",
					RewritePath: "/api/v1/$1",
					TargetHost:  "",
					Enabled:     true,
				},
			},
		},
		{
			name:         "User Path Rewrite",
			path:         "/user/123",
			expectedPath: "/profile/123",
			rewriteRules: []RewriteRule{
				{
					ID:          "rewrite_2",
					Name:        "User Rewrite",
					MatchPath:   "/user/(.*)",
					RewritePath: "/profile/$1",
					TargetHost:  "",
					Enabled:     true,
				},
			},
		},
		{
			name:            "Path Rewrite with Host Change",
			path:            "/user/456",
			expectedPath:    "/profile/456",
			expectedNewHost: "profile-service.example.com",
			rewriteRules: []RewriteRule{
				{
					ID:          "rewrite_3",
					Name:        "User Rewrite with Host",
					MatchPath:   "/user/(.*)",
					RewritePath: "/profile/$1",
					TargetHost:  "profile-service.example.com",
					Enabled:     true,
				},
			},
		},
		{
			name:         "No Matching Rewrite",
			path:         "/random/path",
			expectedPath: "/random/path", // 应保持不变
			rewriteRules: []RewriteRule{
				{
					ID:          "rewrite_4",
					Name:        "Test Rewrite",
					MatchPath:   "/api/test/(.*)",
					RewritePath: "/api/v1/$1",
					TargetHost:  "",
					Enabled:     true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 设置测试配置
			proxy.config.Rewrites = tc.rewriteRules
			proxy.config.Routes = []RouteRule{} // 清空路由规则

			// 重新编译重写规则的正则表达式
			proxy.rewriteRegexps = make(map[string]*regexp.Regexp)
			for _, rule := range tc.rewriteRules {
				if rule.Enabled {
					if regexp, err := regexp.Compile(rule.MatchPath); err == nil {
						proxy.rewriteRegexps[rule.ID] = regexp
					}
				}
			}

			// 创建测试请求
			req := httptest.NewRequest("GET", fmt.Sprintf("http://example.com%s", tc.path), nil)

			// 应用重写规则
			proxy.applyRewrite(req)

			// 检查路径是否正确重写
			if req.URL.Path != tc.expectedPath {
				t.Errorf("Expected path %s, got %s", tc.expectedPath, req.URL.Path)
			}

			// 如果测试用例指定了期望的新主机，则检查主机是否被正确更改
			if tc.expectedNewHost != "" {
				if req.Host != tc.expectedNewHost {
					t.Errorf("Expected host %s, got %s", tc.expectedNewHost, req.Host)
				}
				if req.URL.Host != tc.expectedNewHost {
					t.Errorf("Expected URL host %s, got %s", tc.expectedNewHost, req.URL.Host)
				}
			}
		})
	}
}

// TestIntegrationRouteAndRewrite 测试路由和重写规则的集成
func TestIntegrationRouteAndRewrite(t *testing.T) {
	proxy := NewDevToolProxy()

	// 设置路由和重写规则
	proxy.config.Routes = []RouteRule{
		{
			ID:              "route_1",
			Name:            "Test Route",
			MatchHost:       "api.example.com",
			MatchPathPrefix: "/api",
			TargetHost:      "api-dev.example.com",
			TargetPort:      443,
			Enabled:         true,
		},
	}

	proxy.config.Rewrites = []RewriteRule{
		{
			ID:          "rewrite_1",
			Name:        "API Version Rewrite",
			MatchPath:   "/api/test/(.*)",
			RewritePath: "/api/v1/$1",
			TargetHost:  "",
			Enabled:     true,
		},
	}

	// 重新编译重写规则的正则表达式
	proxy.rewriteRegexps = make(map[string]*regexp.Regexp)
	for _, rule := range proxy.config.Rewrites {
		if rule.Enabled {
			if regexp, err := regexp.Compile(rule.MatchPath); err == nil {
				proxy.rewriteRegexps[rule.ID] = regexp
			}
		}
	}

	// 创建测试请求
	req := httptest.NewRequest("GET", "http://api.example.com/api/test/users", nil)
	req.Host = "api.example.com"

	// 应用重写规则（应该先执行）
	proxy.applyRewrite(req)

	// 检查路径是否被重写
	expectedPath := "/api/v1/users"
	if req.URL.Path != expectedPath {
		t.Errorf("Expected path %s after rewrite, got %s", expectedPath, req.URL.Path)
	}

	// 应用路由规则（应该后执行）
	proxy.applyRouting(req)

	// 检查最终结果
	expectedHost := "api-dev.example.com:443"
	if req.Host != expectedHost {
		t.Errorf("Expected host %s after routing, got %s", expectedHost, req.Host)
	}
}

// TestConfigLoading 测试配置加载功能
func TestConfigLoading(t *testing.T) {
	// 测试从JSON字符串加载配置
	jsonConfig := `{
    "port": "9090",
    "routes": [
      {
        "id": "route_1",
        "name": "Test Route",
        "match_host": "test.com",
        "match_path_prefix": "/api",
        "target_host": "test-api.com",
        "target_port": 8080,
        "enabled": true
      }
    ],
    "rewrites": [
      {
        "id": "rewrite_1",
        "name": "Test Rewrite",
        "match_path": "/old/(.*)",
        "rewrite_path": "/new/$1",
        "target_host": "",
        "enabled": true
      }
    ],
    "origins": [
      {
        "id": "origin_1",
        "name": "Test Origin",
        "origin": "test.com",
        "enabled": true,
        "record_body": true
      }
    ],
    "record_requests": true,
    "max_request_history": 500
  }`

	var config Config
	err := json.Unmarshal([]byte(jsonConfig), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// 验证配置加载
	if config.Port != "9090" {
		t.Errorf("Expected port 9090, got %s", config.Port)
	}

	if len(config.Routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(config.Routes))
	}

	if len(config.Rewrites) != 1 {
		t.Errorf("Expected 1 rewrite, got %d", len(config.Rewrites))
	}

	if len(config.Origins) != 1 {
		t.Errorf("Expected 1 origin, got %d", len(config.Origins))
	}

	if config.RecordRequests != true {
		t.Errorf("Expected RecordRequests true, got %v", config.RecordRequests)
	}

	if config.MaxRequestHistory != 500 {
		t.Errorf("Expected MaxRequestHistory 500, got %d", config.MaxRequestHistory)
	}
}

// TestRequestTracking 测试请求跟踪功能
func TestRequestTracking(t *testing.T) {
	proxy := NewDevToolProxy()
	proxy.config.RecordRequests = true

	// 创建一个带有请求体的测试请求
	body := `{"test": "data", "value": 123}`
	req := httptest.NewRequest("POST", "http://example.com/api/test", strings.NewReader(body))

	// 跟踪请求
	trackedReq := trackRequestBody(req)

	// 确保请求体被读取
	_, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Error reading request body: %v", err)
	}

	// 等待请求体读取完成
	trackedReq.Body.WaitForReadOrTimeout(1 * time.Second)

	// 检查请求体内容是否正确跟踪
	content := string(trackedReq.Body.Content())
	if content != body {
		t.Errorf("Expected request body %s, got %s", body, content)
	}

	// 检查读取是否完成
	if !trackedReq.IsReadDone {
		t.Errorf("Expected request to be read done")
	}
}

// TestResponseTracking 测试响应跟踪功能
func TestResponseTracking(t *testing.T) {
	proxy := NewDevToolProxy()
	proxy.config.RecordRequests = true

	// 创建一个带有响应体的测试响应
	body := `{"result": "success", "code": 200}`
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	// 跟踪响应
	trackedResp := trackResponseBody(resp)

	// 确保响应体被读取
	_, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}

	// 等待响应体读取完成
	trackedResp.Body.WaitForReadOrTimeout(1 * time.Second)

	// 检查响应体内容是否正确跟踪
	content := string(trackedResp.Body.Content())
	if content != body {
		t.Errorf("Expected response body %s, got %s", body, content)
	}

	// 检查读取是否完成
	if !trackedResp.IsReadDone {
		t.Errorf("Expected response to be read done")
	}
}

// TestHTTPTextConversion 测试HTTP协议文本转换功能
func TestHTTPTextConversion(t *testing.T) {
	// 测试请求转换
	req := httptest.NewRequest("POST", "http://example.com/api/test?param=value", strings.NewReader(`{"data": "test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")

	requestText := httpTextFromRequest(req)
	if !strings.Contains(requestText, "POST /api/test?param=value HTTP/1.1") {
		t.Errorf("Request text does not contain expected request line")
	}
	if !strings.Contains(requestText, "Content-Type: application/json") {
		t.Errorf("Request text does not contain Content-Type header")
	}
	if !strings.Contains(requestText, "Authorization: Bearer token123") {
		t.Errorf("Request text does not contain Authorization header")
	}

	// 测试响应转换
	resp := &http.Response{
		Status:     "OK",
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		Header:     make(http.Header),
		Body:       nil,
	}
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("Server", "Test-Server")

	responseText := httpTextFromResponse(resp)
	if !strings.Contains(responseText, "HTTP/1.1 200 OK") {
		t.Errorf("Response text does not contain expected status line")
	}
	if !strings.Contains(responseText, "Content-Type: application/json") {
		t.Errorf("Response text does not contain Content-Type header")
	}
	if !strings.Contains(responseText, "Server: Test-Server") {
		t.Errorf("Response text does not contain Server header")
	}
}

// TestOriginTracking 测试基于Origin的跟踪功能
func TestOriginTracking(t *testing.T) {
	proxy := NewDevToolProxy()

	// 测试配置中没有Origin规则的情况
	proxy.config.Origins = []OriginRule{}
	req := httptest.NewRequest("GET", "http://example.com", nil)
	if !proxy.isOriginTracked(req) {
		t.Error("Expected all requests to be tracked when no origin rules are configured")
	}

	// 测试有Origin规则的情况
	proxy.config.Origins = []OriginRule{
		{
			ID:         "origin_1",
			Name:       "Test Origin",
			Origin:     "example.com",
			Enabled:    true,
			RecordBody: true,
		},
		{
			ID:         "origin_2",
			Name:       "Another Origin",
			Origin:     "another.com",
			Enabled:    false, // This should be ignored
			RecordBody: true,
		},
	}

	// 测试匹配的Origin
	req.Header.Set("Origin", "https://example.com")
	if !proxy.isOriginTracked(req) {
		t.Error("Expected request with matching Origin to be tracked")
	}

	// 测试不匹配的Origin
	req.Header.Set("Origin", "https://unmatched.com")
	if proxy.isOriginTracked(req) {
		t.Error("Expected request with non-matching Origin to not be tracked")
	}

	// 测试禁用的Origin规则
	req.Header.Set("Origin", "https://another.com")
	if proxy.isOriginTracked(req) {
		t.Error("Expected request with disabled Origin rule to not be tracked")
	}

	// 测试Referer头（当Origin头不存在时）
	req.Header.Del("Origin")
	req.Header.Set("Referer", "https://example.com/page")
	if !proxy.isOriginTracked(req) {
		t.Error("Expected request with matching Referer to be tracked")
	}
}
