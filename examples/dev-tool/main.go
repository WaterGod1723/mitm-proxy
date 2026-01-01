package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/WaterGod1723/mitm-proxy/core"
)

// RouteRule 表示路由规则
type RouteRule struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	MatchHost       string `json:"match_host"`
	MatchPathPrefix string `json:"match_path_prefix"`
	TargetHost      string `json:"target_host"`
	TargetPort      int    `json:"target_port"`
	Enabled         bool   `json:"enabled"`
}

// RewriteRule 表示路径重写规则
type RewriteRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MatchPath   string `json:"match_path"`
	RewritePath string `json:"rewrite_path"`
	TargetHost  string `json:"target_host"`
	Enabled     bool   `json:"enabled"`
}

// OriginRule 表示基于Origin的页面配置规则
type OriginRule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Origin     string `json:"origin"`
	Enabled    bool   `json:"enabled"`
	RecordBody bool   `json:"record_body"`
}

// Config 代理配置
type Config struct {
	Port              string        `json:"port"`
	Routes            []RouteRule   `json:"routes"`
	Rewrites          []RewriteRule `json:"rewrites"`
	Origins           []OriginRule  `json:"origins"`
	RecordRequests    bool          `json:"record_requests"`
	MaxRequestHistory int           `json:"max_request_history"`
}

// StreamTrackingBody 用于流式跟踪请求/响应体
type StreamTrackingBody struct {
	content           []byte
	reader            io.Reader
	writer            *io.PipeWriter
	mutex             sync.RWMutex
	isReadDone        bool
	readDoneCallbacks []func()
	closeChan         chan struct{} // 用于通知关闭
	closeOnce         sync.Once     // 确保closeChan只关闭一次
}

func NewStreamTrackingBody(originalBody io.ReadCloser) (*StreamTrackingBody, io.ReadCloser) {
	reader, writer := io.Pipe()
	tracker := &StreamTrackingBody{
		reader:            reader,
		writer:            writer,
		isReadDone:        false,
		readDoneCallbacks: make([]func(), 0),
		closeChan:         make(chan struct{}),
		closeOnce:         sync.Once{},
	}

	// 启动 goroutine 来复制数据流
	go func() {
		var buf bytes.Buffer
		// 将原始 body 复制到 pipe writer，同时写入 buffer 以保存内容
		_, err := io.Copy(io.MultiWriter(writer, &buf), originalBody)
		originalBody.Close()
		writer.CloseWithError(err) // 关闭 writer 并传递可能的错误

		tracker.mutex.Lock()
		tracker.content = buf.Bytes()
		tracker.isReadDone = true
		callbacks := tracker.readDoneCallbacks
		tracker.readDoneCallbacks = nil
		tracker.closeOnce.Do(func() {
			close(tracker.closeChan) // 关闭通知通道
		})
		tracker.mutex.Unlock()

		// 触发所有回调
		for _, callback := range callbacks {
			callback()
		}
	}()

	return tracker, struct {
		io.Reader
		io.Closer
	}{
		Reader: reader,
		Closer: reader,
	}
}

func (stb *StreamTrackingBody) Read(p []byte) (n int, err error) {
	return stb.reader.Read(p)
}

func (stb *StreamTrackingBody) Content() []byte {
	stb.mutex.RLock()
	defer stb.mutex.RUnlock()
	return stb.content
}

func (stb *StreamTrackingBody) IsReadCompleted() bool {
	stb.mutex.RLock()
	defer stb.mutex.RUnlock()
	return stb.isReadDone
}

func (stb *StreamTrackingBody) OnReadDone(callback func()) {
	stb.mutex.Lock()
	defer stb.mutex.Unlock()
	if stb.isReadDone {
		// 如果已经读取完成，直接调用回调
		callback()
	} else {
		// 否则添加到回调列表
		stb.readDoneCallbacks = append(stb.readDoneCallbacks, callback)
	}
}

// MarkReadDone 手动标记读取完成，用于流式响应
func (stb *StreamTrackingBody) MarkReadDone() {
	stb.mutex.Lock()
	defer stb.mutex.Unlock()
	if !stb.isReadDone {
		stb.isReadDone = true
		callbacks := stb.readDoneCallbacks
		stb.readDoneCallbacks = nil
		stb.closeOnce.Do(func() {
			close(stb.closeChan) // 关闭通知通道
		})
		stb.mutex.Unlock() // 临时释放锁以调用回调
		// 触发所有回调
		for _, callback := range callbacks {
			callback()
		}
		stb.mutex.Lock() // 重新获取锁
	}
}

// WaitForReadOrTimeout 等待读取完成或超时
func (stb *StreamTrackingBody) WaitForReadOrTimeout(timeout time.Duration) {
	if stb.IsReadCompleted() {
		return
	}
	select {
	case <-stb.closeChan:
		// 读取已完成
	case <-time.After(timeout):
		// 超时，手动标记为完成
		stb.MarkReadDone()
	}
}

// TrackedRequest 用于跟踪请求
type TrackedRequest struct {
	Request    *http.Request
	Body       *StreamTrackingBody
	IsReadDone bool
	ReadMutex  sync.RWMutex
}

// TrackedResponse 用于跟踪响应
type TrackedResponse struct {
	Response   *http.Response
	Body       *StreamTrackingBody
	IsReadDone bool
	ReadMutex  sync.RWMutex
}

// RequestRecord 请求记录
type RequestRecord struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	RequestText  string    `json:"request_text"`  // HTTP请求协议文本
	ResponseText string    `json:"response_text"` // HTTP响应协议文本
	RequestTime  time.Time `json:"request_time"`  // 记录请求发起时间
	Duration     string    `json:"duration"`
}

// PendingRequest 用于跟踪未完成的请求
type PendingRequest struct {
	Request     *TrackedRequest
	RequestTime time.Time
}

// RequestResponseTracker 请求响应跟踪器
type RequestResponseTracker struct {
	trackedRequests  map[string]*TrackedRequest
	trackedResponses map[string]*TrackedResponse
	pendingRequests  map[string]*PendingRequest // 使用请求的唯一ID映射到待完成的请求
	mutex            sync.RWMutex
}

// DevToolProxy 开发工具代理
type DevToolProxy struct {
	config         *Config
	mitm           *core.Container
	requestRecords []RequestRecord
	recordsMutex   sync.RWMutex
	rewriteRegexps map[string]*regexp.Regexp
	tracker        *RequestResponseTracker
}

// NewDevToolProxy 创建新的开发工具代理
func NewDevToolProxy() *DevToolProxy {
	config := &Config{
		Port:              "8080",
		Routes:            []RouteRule{},
		Rewrites:          []RewriteRule{},
		Origins:           []OriginRule{},
		RecordRequests:    true,
		MaxRequestHistory: 1000,
	}

	// 尝试从配置文件加载
	if configData, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(configData, config); err != nil {
			log.Printf("Error parsing config: %v", err)
		}
	}

	proxy := &DevToolProxy{
		config:         config,
		mitm:           core.NewMITM(),
		requestRecords: make([]RequestRecord, 0),
		rewriteRegexps: make(map[string]*regexp.Regexp),
		tracker: &RequestResponseTracker{
			trackedRequests:  make(map[string]*TrackedRequest),
			trackedResponses: make(map[string]*TrackedResponse),
			pendingRequests:  make(map[string]*PendingRequest),
		},
	}

	// 编译重写规则的正则表达式
	for _, rule := range config.Rewrites {
		if rule.Enabled {
			if regexp, err := regexp.Compile(rule.MatchPath); err == nil {
				proxy.rewriteRegexps[rule.ID] = regexp
			}
		}
	}

	return proxy
}

// applyRouting 应用路由规则
func (dtp *DevToolProxy) applyRouting(req *http.Request) {
	for _, rule := range dtp.config.Routes {
		if !rule.Enabled {
			continue
		}

		// 检查主机匹配
		if req.Host == rule.MatchHost {
			// 检查路径前缀匹配
			if strings.HasPrefix(req.URL.Path, rule.MatchPathPrefix) {
				// 应用路由规则
				newHost := fmt.Sprintf("%s:%d", rule.TargetHost, rule.TargetPort)
				req.Host = newHost
				req.URL.Host = newHost
				// 使用url包处理URL编码
				req.URL = &url.URL{ // 重新构建URL以确保正确性
					Scheme:   req.URL.Scheme,
					Host:     newHost,
					Path:     req.URL.Path,
					RawQuery: req.URL.RawQuery,
				}
				log.Printf("Routing request from %s to %s", rule.MatchHost, newHost)
				break
			}
		}
	}
}

// applyRewrite 应用路径重写规则
func (dtp *DevToolProxy) applyRewrite(req *http.Request) {
	for _, rule := range dtp.config.Rewrites {
		if !rule.Enabled {
			continue
		}

		regexp, exists := dtp.rewriteRegexps[rule.ID]
		if !exists {
			continue
		}

		// 检查路径匹配
		if regexp.MatchString(req.URL.Path) {
			// 应用路径重写
			newPath := regexp.ReplaceAllString(req.URL.Path, rule.RewritePath)
			req.URL.Path = newPath
			req.URL.RawPath = newPath // 更新原始路径

			// 如果指定了目标主机，则更新主机
			if rule.TargetHost != "" {
				req.Host = rule.TargetHost
				req.URL.Host = req.Host
			}

			log.Printf("Rewriting path from %s to %s", req.URL.Path, newPath)
			break
		}
	}
}

// trackRequestBody 跟踪请求体
func trackRequestBody(req *http.Request) *TrackedRequest {
	if req.Body == nil {
		// 创建一个空的 StreamTrackingBody
		tracker := &StreamTrackingBody{
			content:           []byte{},
			reader:            bytes.NewReader([]byte{}),
			isReadDone:        true,
			readDoneCallbacks: make([]func(), 0),
			closeChan:         make(chan struct{}),
			closeOnce:         sync.Once{},
		}
		return &TrackedRequest{
			Request:    req,
			Body:       tracker,
			IsReadDone: true,
		}
	}

	// 使用流式跟踪器
	tracker, newBody := NewStreamTrackingBody(req.Body)
	req.Body = newBody

	// 创建 TrackedRequest 并设置回调来更新 IsReadDone
	trackedReq := &TrackedRequest{
		Request:    req,
		Body:       tracker,
		IsReadDone: false, // 初始状态为未完成
	}

	// 当读取完成时，更新 IsReadDone 状态
	tracker.OnReadDone(func() {
		trackedReq.ReadMutex.Lock()
		defer trackedReq.ReadMutex.Unlock()
		trackedReq.IsReadDone = true
	})

	return trackedReq
}

// trackResponseBody 跟踪响应体
func trackResponseBody(resp *http.Response) *TrackedResponse {
	if resp.Body == nil {
		// 创建一个空的 StreamTrackingBody
		tracker := &StreamTrackingBody{
			content:           []byte{},
			reader:            bytes.NewReader([]byte{}),
			isReadDone:        true,
			readDoneCallbacks: make([]func(), 0),
			closeChan:         make(chan struct{}),
			closeOnce:         sync.Once{},
		}
		return &TrackedResponse{
			Response:   resp,
			Body:       tracker,
			IsReadDone: true,
		}
	}

	// 使用流式跟踪器
	tracker, newBody := NewStreamTrackingBody(resp.Body)
	resp.Body = newBody

	// 创建 TrackedResponse 并设置回调来更新 IsReadDone
	trackedResp := &TrackedResponse{
		Response:   resp,
		Body:       tracker,
		IsReadDone: false, // 初始状态为未完成
	}

	// 当读取完成时，更新 IsReadDone 状态
	tracker.OnReadDone(func() {
		trackedResp.ReadMutex.Lock()
		defer trackedResp.ReadMutex.Unlock()
		trackedResp.IsReadDone = true
	})

	return trackedResp
}

// cloneHeaders 复制HTTP头部
func cloneHeaders(h http.Header) http.Header {
	clone := make(http.Header)
	for k, v := range h {
		clone[k] = make([]string, len(v))
		copy(clone[k], v)
	}
	return clone
}

// httpTextFromRequest 将HTTP请求转换为协议文本格式
func httpTextFromRequest(req *http.Request) string {
	var buf bytes.Buffer

	// 写入请求行
	uri := req.URL.RequestURI()
	if uri == "" {
		uri = "/"
	}
	buf.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, uri))

	// 写入请求头
	for name, values := range req.Header {
		for _, value := range values {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", name, value))
		}
	}

	// 添加空行分隔头和体
	buf.WriteString("\r\n")

	return buf.String()
}

// httpTextFromResponse 将HTTP响应转换为协议文本格式
func httpTextFromResponse(resp *http.Response) string {
	var buf bytes.Buffer

	// 写入状态行
	// 从状态字符串中提取状态文本部分（去掉状态码部分）
	buf.WriteString(fmt.Sprintf("HTTP/1.1 %s\r\n", resp.Status))

	// 写入响应头
	for name, values := range resp.Header {
		for _, value := range values {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", name, value))
		}
	}

	// 添加空行分隔头和体
	buf.WriteString("\r\n")

	return buf.String()
}

// buildRequestText 构建HTTP请求协议文本
func buildRequestText(req *http.Request, reqBody []byte) string {
	var buf bytes.Buffer

	// 写入请求部分
	buf.WriteString(httpTextFromRequest(req))

	// 添加请求体（如果存在）
	if len(reqBody) > 0 {
		buf.Write(reqBody)
	}

	return buf.String()
}

// buildResponseText 构建HTTP响应协议文本
func buildResponseText(resp *http.Response, respBody []byte) string {
	var buf bytes.Buffer

	// 写入响应部分
	buf.WriteString(httpTextFromResponse(resp))

	// 添加响应体（如果存在）
	if len(respBody) > 0 {
		buf.Write(respBody)
	}

	return buf.String()
}

// isOriginTracked 检查请求的Origin是否在跟踪配置中
func (dtp *DevToolProxy) isOriginTracked(req *http.Request) bool {
	// 如果没有配置Origin规则，则跟踪所有请求
	if len(dtp.config.Origins) == 0 {
		return true
	}

	// 获取请求的Origin头
	origin := req.Header.Get("Origin")
	if origin == "" {
		// 如果没有Origin头，检查Referer
		origin = req.Header.Get("Referer")
	}

	// 检查是否有匹配的Origin规则
	for _, rule := range dtp.config.Origins {
		if rule.Enabled && strings.Contains(origin, rule.Origin) {
			return true
		}
	}

	return false
}

// trackRequestOnly 仅跟踪请求
func (dtp *DevToolProxy) trackRequestOnly(req *http.Request) {
	if !dtp.config.RecordRequests {
		return
	}

	// 检查Origin是否在跟踪范围内
	if !dtp.isOriginTracked(req) {
		return
	}

	// 只有在请求体不为 nil 时才进行跟踪
	if req.Body != nil {
		trackedReq := trackRequestBody(req)
		requestID := fmt.Sprintf("%d", time.Now().UnixNano()) // 使用时间戳作为唯一ID

		dtp.tracker.mutex.Lock()
		defer dtp.tracker.mutex.Unlock()

		// 存储待完成的请求
		dtp.tracker.pendingRequests[requestID] = &PendingRequest{
			Request:     trackedReq,
			RequestTime: time.Now(),
		}

		// 启动一个goroutine来处理请求体读取完成的逻辑
		go func() {
			// 等待请求体读取完成或超时
			trackedReq.Body.WaitForReadOrTimeout(30 * time.Second)

			// 将完成的请求移动到trackedRequests
			dtp.tracker.mutex.Lock()
			defer dtp.tracker.mutex.Unlock()
			dtp.tracker.trackedRequests[requestID] = trackedReq
			// 从pending列表中删除
			delete(dtp.tracker.pendingRequests, requestID)
		}()
	}
}

// trackResponseOnly 仅跟踪响应
func (dtp *DevToolProxy) trackResponseOnly(req *http.Request, resp *http.Response) {
	if !dtp.config.RecordRequests {
		return
	}

	// 检查Origin是否在跟踪范围内
	if !dtp.isOriginTracked(req) {
		return
	}

	// 尝试找到对应的请求
	requestID := "" // 在实际实现中，我们需要一个方法来关联请求和响应

	// 这里我们简单地使用URL和时间戳来查找最近的请求
	dtp.tracker.mutex.RLock()
	for id, pendingReq := range dtp.tracker.pendingRequests {
		// 简单比较请求的URL和方法
		if pendingReq.Request.Request.URL.String() == req.URL.String() &&
			pendingReq.Request.Request.Method == req.Method {
			requestID = id
			break
		}
	}
	dtp.tracker.mutex.RUnlock()

	trackedResp := trackResponseBody(resp)

	// 启动一个goroutine来处理响应记录，避免无限等待流式响应
	go func() {
		// 检查响应是否是流式响应（如server-sent events）
		contentType := resp.Header.Get("Content-Type")
		contentEncoding := resp.Header.Get("Content-Encoding")
		transferEncoding := resp.Header.Get("Transfer-Encoding")
		isStreamResponse := strings.Contains(contentType, "text/event-stream") ||
			strings.Contains(contentType, "application/x-mpegURL") ||
			strings.Contains(contentEncoding, "chunked") ||
			strings.Contains(transferEncoding, "chunked")

		if isStreamResponse {
			// 对于流式响应，设置一个合理的超时时间，而不是无限等待
			trackedResp.Body.WaitForReadOrTimeout(5 * time.Second)
		} else {
			// 对于普通响应，等待读取完成
			trackedResp.Body.OnReadDone(func() {})
			// 等待完成或超时
			trackedResp.Body.WaitForReadOrTimeout(30 * time.Second)
		}

		// 记录响应数据
		dtp.recordsMutex.Lock()
		defer dtp.recordsMutex.Unlock()

		// 查找对应的请求
		var requestTime time.Time
		var requestBody []byte

		dtp.tracker.mutex.RLock()
		if pendingReq, exists := dtp.tracker.pendingRequests[requestID]; exists {
			// 如果请求仍在pending状态，使用pending信息
			requestTime = pendingReq.RequestTime
			requestBody = pendingReq.Request.Body.Content()
			// 从pending列表中删除
			delete(dtp.tracker.pendingRequests, requestID)
		} else if trackedReq, exists := dtp.tracker.trackedRequests[requestID]; exists {
			// 如果请求已完成，使用已完成的信息
			requestBody = trackedReq.Body.Content()
			// 从tracked列表中删除
			delete(dtp.tracker.trackedRequests, requestID)
		} else {
			// 没有找到对应的请求，使用当前响应的信息
			requestTime = time.Now()
			// 尝试获取请求体内容
			if req.Body != nil {
				// 由于无法获取未跟踪的请求体内容，我们只能获取空值
				requestBody = []byte("")
			} else {
				requestBody = []byte("")
			}
		}
		dtp.tracker.mutex.RUnlock()

		// 创建请求记录，使用分离的HTTP协议文本格式
		respBody := trackedResp.Body.Content()
		record := RequestRecord{
			ID:           fmt.Sprintf("%d", time.Now().UnixNano()),
			Timestamp:    time.Now(),
			RequestText:  buildRequestText(req, requestBody),
			ResponseText: buildResponseText(resp, respBody),
			RequestTime:  requestTime,
			Duration:     time.Since(requestTime).String(),
		}

		dtp.requestRecords = append(dtp.requestRecords, record)

		// 限制历史记录数量
		if len(dtp.requestRecords) > dtp.config.MaxRequestHistory {
			dtp.requestRecords = dtp.requestRecords[len(dtp.requestRecords)-dtp.config.MaxRequestHistory:]
		}
	}()
}

// searchRequests 搜索请求记录
func (dtp *DevToolProxy) searchRequests(query string) []RequestRecord {
	dtp.recordsMutex.RLock()
	defer dtp.recordsMutex.RUnlock()

	if query == "" {
		return dtp.requestRecords
	}

	var results []RequestRecord
	lowerQuery := strings.ToLower(query)

	for _, record := range dtp.requestRecords {
		// 在请求和响应协议文本中搜索查询内容
		match := strings.Contains(strings.ToLower(record.RequestText), lowerQuery) ||
			strings.Contains(strings.ToLower(record.ResponseText), lowerQuery)

		if match {
			results = append(results, record)
		}
	}

	return results
}

// setupRequestProcessing 设置请求处理
func (dtp *DevToolProxy) setupRequestProcessing() {
	dtp.mitm.ProcessRequest(func(req *http.Request) core.ResponseWriteFunc {
		req.Header.Set("Accept-Encoding", "identity")
		// 应用路由规则
		dtp.applyRouting(req)

		// 应用路径重写规则
		dtp.applyRewrite(req)

		// 跟踪请求 - 在所有修改完成后进行跟踪
		dtp.trackRequestOnly(req)

		return nil
	})
}

// setupResponseProcessing 设置响应处理
func (dtp *DevToolProxy) setupResponseProcessing() {
	dtp.mitm.ProcessResponse(func(resp *http.Response) core.ResponseWriteFunc {
		// 记录响应，关联之前的请求
		if resp.Request != nil && dtp.config.RecordRequests {
			dtp.trackResponseOnly(resp.Request, resp)
		}

		return nil
	})
}

// setupWebInterface 设置Web界面
func (dtp *DevToolProxy) setupWebInterface() {
	// 获取所有请求记录
	dtp.mitm.HandleFunc("/api/requests", func(w *core.ResponseWriter, r *http.Request) {
		// 设置CORS头部
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == "OPTIONS" {
			w.SetStatus(http.StatusOK)
			w.Write([]byte(""))
			return
		}

		// 获取查询参数
		query := r.URL.Query().Get("q")
		requests := dtp.searchRequests(query)

		// 返回JSON格式的请求记录
		w.Header().Set("Content-Type", "application/json")
		jsonData, _ := json.Marshal(requests)
		w.SetStatus(http.StatusOK)
		w.Write(jsonData)
	})

	// 清除请求记录
	dtp.mitm.HandleFunc("/api/clear-requests", func(w *core.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == "OPTIONS" {
			w.SetStatus(http.StatusOK)
			w.Write([]byte(""))
			return
		}

		if r.Method == "POST" {
			dtp.recordsMutex.Lock()
			dtp.requestRecords = make([]RequestRecord, 0)
			dtp.recordsMutex.Unlock()

			w.SetStatus(http.StatusOK)
			w.Write([]byte("Request records cleared successfully"))
		} else {
			w.SetStatus(http.StatusMethodNotAllowed)
			w.Write([]byte("Method not allowed"))
		}
	})

	// 获取配置
	dtp.mitm.HandleFunc("/api/config", func(w *core.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == "OPTIONS" {
			w.SetStatus(http.StatusOK)
			w.Write([]byte(""))
			return
		}

		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			jsonData, _ := json.Marshal(dtp.config)
			w.SetStatus(http.StatusOK)
			w.Write(jsonData)
		} else if r.Method == "POST" {
			// 更新配置
			body, _ := io.ReadAll(r.Body)
			var newConfig Config
			if err := json.Unmarshal(body, &newConfig); err != nil {
				w.SetStatus(http.StatusBadRequest)
				w.Write([]byte(fmt.Sprintf("Error parsing config: %v", err)))
				return
			}

			// 更新配置
			dtp.config = &newConfig

			// 保存到文件
			configData, _ := json.MarshalIndent(newConfig, "", "  ")
			os.WriteFile("config.json", configData, 0644)

			w.SetStatus(http.StatusOK)
			w.Write([]byte("Config updated successfully"))
		}
	})

	// Web界面主页
	dtp.mitm.HandleFunc("/", func(w *core.ResponseWriter, r *http.Request) {
		// 返回HTML界面
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.SetStatus(http.StatusOK)

		// 读取HTML文件内容
		htmlContent, err := os.ReadFile("index.html")
		if err != nil {
			htmlContent = []byte("<html><body><h1>Unable to load UI</h1><p>Please ensure index.html is in the same directory.</p></body></html>")
		}
		html := string(htmlContent)

		w.Write([]byte(html))
	})
}

// Start 启动代理
func (dtp *DevToolProxy) Start(addr string) {
	// 设置请求和响应处理
	dtp.setupRequestProcessing()
	dtp.setupResponseProcessing()

	// 设置Web界面
	dtp.setupWebInterface()

	// 启动MITM代理
	log.Printf("Dev Tool Proxy started on port %s", dtp.config.Port)
	dtp.mitm.Start(addr)
}

func main() {
	addr := flag.String("addr", ":8080", "代理服务地址")
	flag.Parse()

	proxy := NewDevToolProxy()
	proxy.Start(*addr)
}
