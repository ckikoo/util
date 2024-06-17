package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	hubHost         = "registry-1.docker.io"
	authURL         = "auth.docker.io"
	blacklistTime   = time.Hour
	requestLimit    = 5
	cleanupInterval = time.Minute
)

var (
	requestCounts sync.Map
	blacklist     sync.Map
)

func main() {
	go cleanupBlacklist() // 启动一个goroutine定期清理黑名单
	http.HandleFunc("/", rateLimiter(handleRequest))
	fmt.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func cleanupBlacklist() {
	for {
		time.Sleep(cleanupInterval)
		now := time.Now()
		blacklist.Range(func(key, value interface{}) bool {
			if value.(time.Time).Before(now) {
				blacklist.Delete(key)
			}
			return true
		})
	}
}

func rateLimiter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		// 检查黑名单
		if expireTime, exists := blacklist.Load(ip); exists {
			if time.Now().Before(expireTime.(time.Time)) {
				http.Error(w, "IP temporarily blacklisted", http.StatusForbidden)
				return
			} else {
				blacklist.Delete(ip)
			}
		}

		// 获取当前时间的秒级时间戳
		now := time.Now().Unix()

		// 从请求计数器中获取当前 IP 的请求次数
		value, _ := requestCounts.LoadOrStore(ip, &sync.Map{})
		userRequests := value.(*sync.Map)

		// 获取当前时间戳的请求次数
		count, _ := userRequests.LoadOrStore(now, new(int))
		currentCount := count.(*int)

		// 增加当前时间戳的请求次数
		*currentCount++


		// 超过阈值时将 IP 加入黑名单
		if *currentCount > requestLimit {
			blacklist.Store(ip, time.Now().Add(blacklistTime))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		// 清理过期的请求计数
		userRequests.Range(func(key, value interface{}) bool {
			if key.(int64) < now {
				userRequests.Delete(key)
			}
			return true
		})

		next(w, r)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 设置跨域权限
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 检查是否为预检请求
	if r.Method == "OPTIONS" {
		handlePreflight(w, r)
		return
	}

	// 处理 token 请求
	if strings.HasPrefix(r.URL.Path, "/token") {
		handleTokenRequest(w, r)
		return
	}

	// 修改请求目标到 Docker Hub
	proxyURL := &url.URL{
		Scheme: "https",
		Host:   hubHost,
		Path:   r.URL.Path,
	}
	proxyRequest(w, r, proxyURL)
}

func handleTokenRequest(w http.ResponseWriter, r *http.Request) {
	// 构造向认证服务器的请求
	tokenURL := fmt.Sprintf("https://%s%s", authURL, r.URL.RequestURI())
	proxyRequest(w, r, mustParseURL(tokenURL))
}

func handlePreflight(w http.ResponseWriter, r *http.Request) {
	// 设置预检请求响应头
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, TRACE, DELETE, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Max-Age", "1728000")
	w.WriteHeader(http.StatusOK)
}

func proxyRequest(w http.ResponseWriter, r *http.Request, proxyURL *url.URL) {
	// 创建新请求
	proxyReq, err := http.NewRequest(r.Method, proxyURL.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}
	proxyReq.Header = r.Header

	// 发起请求
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Failed to fetch response", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 复制响应头和状态码
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 复制响应体
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Failed to copy response body: %v", err)
	}
}

func mustParseURL(rawurl string) *url.URL {
	parsedURL, err := url.Parse(rawurl)
	if err != nil {
		log.Fatalf("Failed to parse URL: %v", err)
	}
	return parsedURL
}
