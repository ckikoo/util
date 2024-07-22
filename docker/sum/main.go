package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// URLInfo 代表一个URL的详细信息
type URLInfo struct {
	URL          string
	Dead         bool
	Load         int
	ResponseTime float64 // 响应时间，以秒为单位
	Weight       float64 // 动态权重
	mu           sync.Mutex
}

// URLManager 管理URL的CRUD操作和负载均衡
type URLManager struct {
	urls []*URLInfo
	mu   sync.RWMutex
	rand *rand.Rand
}

// NewURLManager 初始化一个URLManager
func NewURLManager() *URLManager {
	return &URLManager{rand: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// AddURL 添加一个新的URL，初始权重为1
func (um *URLManager) AddURL(url string) {
	um.mu.Lock()
	defer um.mu.Unlock()
	um.urls = append(um.urls, &URLInfo{URL: url, Weight: 1})
}

// Get 获取一个可用的URL，使用动态加权最少连接法
func (um *URLManager) Get() string {
	for {
		um.mu.RLock()
		n := len(um.urls)
		if n == 0 {
			um.mu.RUnlock()
			return ""
		}

		var selectedURL *URLInfo
		minLoadRatio := math.MaxFloat64
		startIndex := um.rand.Intn(n) // 引入随机偏移量

		for i := 0; i < n; i++ {
			urlInfo := um.urls[(startIndex+i)%n]
			urlInfo.mu.Lock()
			if !urlInfo.Dead {
				loadRatio := float64(urlInfo.Load) / urlInfo.Weight
				if loadRatio < minLoadRatio {
					if selectedURL != nil {
						selectedURL.mu.Unlock()
					}
					selectedURL = urlInfo
					minLoadRatio = loadRatio
				} else {
					urlInfo.mu.Unlock()
				}
			} else {
				urlInfo.mu.Unlock()
			}
		}

		if selectedURL != nil {
			selectedURL.Load++
			selectedURL.mu.Unlock()
			um.mu.RUnlock()
			return selectedURL.URL
		}

		// 如果所有URL都标记为死亡，尝试恢复它们
		um.mu.RUnlock()
		um.resume()
	}
}

// Done 标记URL已完成使用，并记录响应时间和内容长度
func (um *URLManager) Done(url string, responseTime float64, contentLength int64) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	for _, urlInfo := range um.urls {
		if urlInfo.URL == url {
			urlInfo.mu.Lock()
			if urlInfo.Load > 0 {
				urlInfo.Load--
			}
			// 动态调整权重，考虑响应时间和内容长度
			beta := 0.7 // 权重因子，增加负载的影响
			k := 1e6    // 初始调节单位不同带来的影响
			ratio := float64(contentLength) / responseTime
			if ratio > 1e9 {
				k = 1e3
			} else if ratio > 1e6 {
				k = 1e5
			}

			urlInfo.Weight = beta*float64(urlInfo.Load) + (1-beta)*(float64(contentLength)/responseTime/k)
			urlInfo.mu.Unlock()
			break
		}
	}
}

// MarkDead 标记URL为死亡状态
func (um *URLManager) MarkDead(url string) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	for _, urlInfo := range um.urls {
		if urlInfo.URL == url {
			urlInfo.mu.Lock()
			urlInfo.Dead = true
			urlInfo.Load = 0
			urlInfo.mu.Unlock()
			break
		}
	}
}

// resume 恢复所有标记为死亡的URL
func (um *URLManager) resume() {
	um.mu.Lock()
	defer um.mu.Unlock()
	for _, urlInfo := range um.urls {
		urlInfo.mu.Lock()
		urlInfo.Dead = false
		urlInfo.mu.Unlock()
	}
}

const (
	bufSize  = 64 * 1024 // 64 KB
	cacheDir = "cache"   // 缓存目录
)
const chunkSize = 100 * 1024 * 1024 // 100MB
var glourls URLManager

func main() {
	glourls = *NewURLManager()
	glourls.AddURL("https://yanyu.icu")
	glourls.AddURL("https://hub.rat.dev")
	glourls.AddURL("https://docker.anyhub.us.kg")
	glourls.AddURL("https://docker.chenby.cn")
	// glourls.AddURL("https://dockerproxy.cn")
	// glourls.AddURL("https://dockerhub.jobcher.com")
	// glourls.AddURL("https://dhub.kubesre.xyz")
	glourls.AddURL("https://hub.alduin.net")
	glourls.AddURL("https://docker.jsdelivr.fyi")
	glourls.AddURL("https://dockercf.jsdelivr.fyi")
	glourls.AddURL("https://dockertest.jsdelivr.fyi")

	os.MkdirAll("logs", 0755)
	os.MkdirAll("cache", 0755)
	http.HandleFunc("/", handleRequest)
	fmt.Println("Listening on :23000")
	log.Fatal(http.ListenAndServe(":23000", nil))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {

	// 设置跨域权限
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 检查是否为预检请求
	if r.Method == "OPTIONS" {
		handlePreflight(w, r)
		return
	}

	proxyRequest(w, r)
}

func handlePreflight(w http.ResponseWriter, r *http.Request) {
	// 设置预检请求响应头
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, TRACE, DELETE, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Max-Age", "1728000")
	w.WriteHeader(http.StatusOK)
}

func serveFromCache(w http.ResponseWriter, cacheFilePath string) bool {
	recordFilePath := getRecordFilePath(cacheFilePath)
	if _, err := os.Stat(recordFilePath); os.IsNotExist(err) {
		// 处理未拆分的文件
		cacheFile, err := os.Open(cacheFilePath)
		if err != nil {
			// log.Printf("Failed to open cache file: %v", err)
			return false
		}
		defer cacheFile.Close()

		w.Header().Set("Content-Type", "application/octet-stream") // 设置合适的 MIME 类型
		io.Copy(w, cacheFile)
		return true
	} else {
		// 处理拆分的文件
		recordFile, err := os.Open(recordFilePath)
		if err != nil {
			log.Printf("Failed to open record file: %v", err)
			return false
		}
		defer recordFile.Close()

		var partCount int
		var totalSize int64
		fmt.Fscanf(recordFile, "Parts: %d\nTotalSize: %d\n", &partCount, &totalSize)

		for part := 0; part < partCount; part++ {
			partFilePath := getCacheFilePathWithPart(cacheFilePath, part)
			cacheFile, err := os.Open(partFilePath)
			if err != nil {
				log.Printf("Failed to open cache file part %d: %v", part, err)
				return false
			}
			defer cacheFile.Close()

			_, err = io.Copy(w, cacheFile)
			if err != nil {
				log.Printf("Failed to copy cache file part %d: %v", part, err)
				return false
			}
		}
		return true
	}
}

func proxyRequest(w http.ResponseWriter, r *http.Request) {
	// 创建日志文件
	logFileName := createLogFileName(r.URL.Path)
	cacheFilePath := getCacheFilePath(r.URL.Path)
	// 如果请求路径是缓存路径，则尝试从缓存中读取
	if shouldCache(r.URL.Path) {
		if serveFromCache(w, cacheFilePath) {
			return
		}
	}
	f, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}
	defer f.Close()
	tickets := 0
	for {
		defer func() { tickets++ }()
		if tickets >= 6 {
			return
		}

		defer f.Close()
		logger := log.New(f, "", log.LstdFlags)
		logger.Println("request header print------------------------------------------------")
		for name, values := range r.Header {
			for _, value := range values {
				logger.Printf("%s: %s\n", name, value)
			}
		}
		// 获取动态负载均衡的URL
		targetURL := glourls.Get()
		if targetURL == "" {
			http.Error(w, "No available URLs", http.StatusServiceUnavailable)
			return
		}
		// 修改请求目标
		proxyURL, err := url.Parse(targetURL)
		if err != nil {
			http.Error(w, "Failed to parse target URL", http.StatusInternalServerError)
			return
		}
		proxyURL.Path = r.URL.Path
		proxyURL.RawQuery = r.URL.RawQuery // 保留原始查询参数
		// 创建新请求
		proxyReq, err := http.NewRequest(r.Method, proxyURL.String(), r.Body)
		if err != nil {
			http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
			return
		}
		proxyReq.Header = r.Header

		startTime := time.Now()
		// 发起请求
		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		responseTime := time.Since(startTime).Seconds()
		if err != nil {
			fmt.Printf("err2222: %v\n", err)
			// glourls.MarkDead(proxyURL.Scheme+proxyReq()) // 标记URL为死亡状态
			continue // 尝试使用下一个URL
		}
		fmt.Printf("%v resp.StatusCode: %v\n", targetURL, resp.StatusCode)
		defer resp.Body.Close()

		glourls.Done(proxyURL.String(), responseTime, resp.ContentLength) // 更新URL的负载信息

		if resp.StatusCode == http.StatusNotFound {
			glourls.MarkDead(proxyURL.String()) // 标记URL为死亡状态
			continue                            // 尝试使用下一个URL
		}

		// 复制响应头和状态码
		for name, values := range resp.Header {
			for _, value := range values {
				if name == "Www-Authenticate" {
					w.Header().Add(name, `Bearer realm="http://192.168.xx.xx:23000/token",service="registry.docker.io"`)
				} else {
					w.Header().Add(name, value)
				}

			}
		}
		w.WriteHeader(resp.StatusCode)

		// 复制并打印响应体
		var builder strings.Builder
		buf := make([]byte, bufSize)
		var cacheFile *os.File
		part := 0
		totalReadSize := int64(0)
		split := resp.ContentLength > 100*1024*1024 // 检查是否需要拆分文件
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				builder.Write(buf[:n])
				if shouldCache(proxyURL.Path) {
					if split {
						// 处理拆分文件
						if cacheFile == nil || totalReadSize+int64(n) > chunkSize { // 超过100MB创建新文件
							if cacheFile != nil {
								totalReadSize = 0
								cacheFile.Close()
							}
							cacheFilePath = getCacheFilePathWithPart(proxyURL.Path, part)
							cacheFile, err = os.Create(cacheFilePath)
							if err != nil {
								logger.Printf("Failed to create cache file: %v", err)
								return
							}
							part++
						}
						cacheFile.Write(buf[:n]) // 将响应写入缓存文件
						totalReadSize += int64(n)
					} else {
						// 处理未拆分文件
						if cacheFile == nil {
							cacheFile, err = os.Create(cacheFilePath)
							if err != nil {
								logger.Printf("Failed to create cache file: %v", err)
								return
							}
							defer cacheFile.Close()
						}
						cacheFile.Write(buf[:n])
					}
				}
				_, writeErr := w.Write(buf[:n])
				if writeErr != nil {
					logger.Printf("Failed to write response body: %v", writeErr)
					if shouldCache(proxyURL.Path) {
						os.Remove(cacheFilePath)
					}
					return
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				logger.Printf("Failed to read response body: %v", err)
				if shouldCache(proxyURL.Path) {
					os.Remove(cacheFilePath)
				}
				return
			}
		}
		body := builder.String()
		if shouldCache(proxyURL.Path) {
			if split {
				if cacheFile != nil {
					cacheFile.Close()
				}
				// 创建记录文件
				recordFilePath := getRecordFilePath(proxyURL.Path)
				recordFile, err := os.Create(recordFilePath)
				if err != nil {
					logger.Printf("Failed to create record file: %v", err)
					return
				}
				defer recordFile.Close()
				record := fmt.Sprintf("Parts: %d\nTotalSize: %d\n", part, totalReadSize)
				recordFile.Write([]byte(record))
			}

			go checkCacheFileSize(cacheFilePath, resp.Header.Get("Content-Length"), logger)
		}

		logger.Println("response header print------------------------------------------------")
		for name, values := range w.Header() {
			for _, value := range values {
				logger.Printf("%s: %s\n", name, value)
			}
		}

		logger.Printf("body: %v", body)
		return // 成功处理后退出循环
	}
}

func shouldCache(urlPath string) bool {
	return strings.Contains(urlPath, "/blobs/sha256:")
}

func createLogFileName(urlPath string) string {
	// 使用当前时间和 URL 路径创建唯一的日志文件名
	timestamp := time.Now().Format("20060102_150405")
	escapedPath := strings.ReplaceAll(urlPath, "/", "_")
	return fmt.Sprintf("logs/%s_%s.log", timestamp, escapedPath)
}

func getCacheFilePath(urlPath string) string {
	hash := extractHashFromURL(urlPath)
	return filepath.Join(cacheDir, hash+".dat")
}

func getCacheFilePathWithPart(urlPath string, part int) string {
	hash := extractHashFromURL(urlPath)
	return filepath.Join(cacheDir, fmt.Sprintf("%s_part_%d.dat", hash, part))
}

func getRecordFilePath(urlPath string) string {
	hash := extractHashFromURL(urlPath)
	return filepath.Join(cacheDir, fmt.Sprintf("%s_record.txt", hash))
}

func extractHashFromURL(urlPath string) string {
	re := regexp.MustCompile(`sha256:([a-fA-F0-9]+)`)
	matches := re.FindStringSubmatch(urlPath)
	if len(matches) > 1 {
		return matches[1]
	}
	parts := strings.Split(urlPath, "/")
	return parts[len(parts)-1]
}

func checkCacheFileSize(url string, contentLengthStr string, logger *log.Logger) {
	if contentLengthStr != "" {
		contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
		if err == nil {
			fmt.Printf("contentLength: %v\n", contentLength)

			cacheFilePath := getCacheFilePath(url)
			recordFilePath := getRecordFilePath(cacheFilePath)
			if _, err := os.Stat(recordFilePath); os.IsNotExist(err) {
				// 处理未拆分的文件
				cacheFile, err := os.Open(cacheFilePath)
				if err != nil {
					log.Printf("Failed to open cache file: %v", err)
					return
				}
				defer cacheFile.Close()

				buf, err := io.ReadAll(cacheFile)
				if err != nil {
					log.Printf("Failed to read cache file: %v", err)
					return
				}

				sha, err := calculateSHA256(strings.NewReader(string(buf)))
				if err != nil {
					log.Printf("Failed to calculate SHA256: %v", err)
					return
				}

				shas := fmt.Sprintf("%x", sha)
				if shas != extractHashFromURL(url) {
					os.Remove(cacheFilePath)
				}
			} else {
				// 处理拆分的文件
				recordFile, err := os.Open(recordFilePath)
				if err != nil {
					log.Printf("Failed to open record file: %v", err)
					return
				}
				defer recordFile.Close()

				var partCount int
				var totalSize int64
				fmt.Fscanf(recordFile, "Parts: %d\nTotalSize: %d\n", &partCount, &totalSize)
				reader := bytes.Buffer{}

				for part := 0; part < partCount; part++ {
					partFilePath := getCacheFilePathWithPart(cacheFilePath, part)
					cacheFile, err := os.Open(partFilePath)
					if err != nil {
						log.Printf("Failed to open cache file part %d: %v", part, err)
						return
					}
					defer cacheFile.Close()

					content, err := io.ReadAll(cacheFile)
					if err != nil {
						log.Printf("Failed to read cache file part %d: %v", part, err)
						return
					}

					reader.Write(content)
				}

				sha, err := calculateSHA256(&reader)
				if err != nil {
					log.Printf("Failed to calculate SHA256: %v", err)
					return
				}

				shas := fmt.Sprintf("%x", sha)
				if shas != extractHashFromURL(url) {
					for part := 0; part < partCount; part++ {
						partFilePath := getCacheFilePathWithPart(cacheFilePath, part)
						os.Remove(partFilePath)
					}
					os.Remove(recordFilePath)
				}
			}
		}
	}
}
func calculateSHA256(reader io.Reader) ([]byte, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return nil, fmt.Errorf("could not copy file contents to hasher: %v", err)
	}
	return hasher.Sum(nil), nil
}
