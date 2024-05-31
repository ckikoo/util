package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// downloadChunk 下载文件的一个分片
func downloadChunk(url string, headers map[string]string, start, end int64, chunkNum int, filename string, wg *sync.WaitGroup, errChan chan error) {
	defer wg.Done()

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		errChan <- fmt.Errorf("failed to create request: %v", err)
		return
	}

	// 设置请求头
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errChan <- fmt.Errorf("failed to download chunk %d: %v", chunkNum, err)
		return
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("failed to download chunk %d: status code %d", chunkNum, resp.StatusCode)
		return
	}

	// 创建目标文件
	out, err := os.Create(fmt.Sprintf("%s_chunk_%d", filename, chunkNum))
	if err != nil {
		errChan <- fmt.Errorf("failed to create chunk file %d: %v", chunkNum, err)
		return
	}
	defer out.Close()

	// 将响应数据写入文件
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("failed to write chunk file %d: %v", chunkNum, err)
		return
	}

}

// mergeChunks 合并所有分片
func mergeChunks(filename string, totalChunks int) error {
	out, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer out.Close()

	for i := 0; i < totalChunks; i++ {
		chunkFile, err := os.Open(fmt.Sprintf("%s_chunk_%d", filename, i))
		if err != nil {
			return fmt.Errorf("failed to open chunk file %d: %v", i, err)
		}

		_, err = io.Copy(out, chunkFile)
		chunkFile.Close()
		if err != nil {
			return fmt.Errorf("failed to copy chunk file %d: %v", i, err)
		}

		// 删除分片文件
		os.Remove(fmt.Sprintf("%s_chunk_%d", filename, i))
	}

	return nil
}

// getContentLength 获取文件总长度
func getContentLength(url string, headers map[string]string) (int64, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status code %d", resp.StatusCode)
	}

	lengthStr := resp.Header.Get("Content-Length")
	length, err := strconv.ParseInt(lengthStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return length, nil
}

// downloadFile 下载整个文件
func downloadFile(url string, headers map[string]string, filename string) error {
	// 获取文件总长度
	contentLength, err := getContentLength(url, headers)
	if err != nil {
		return fmt.Errorf("failed to get content length: %v", err)
	}

	const chunkSize = 10 * 1024 * 1024 // 1MB
	totalChunks := int(contentLength / chunkSize)
	if contentLength%chunkSize != 0 {
		totalChunks++
	}

	var wg sync.WaitGroup
	errChan := make(chan error, totalChunks)

	for i := 0; i < totalChunks; i++ {
		wg.Add(1)
		start := int64(i) * chunkSize
		end := start + chunkSize - 1
		if end > contentLength-1 {
			end = contentLength - 1
		}

		go downloadChunk(url, headers, start, end, i, filename, &wg, errChan)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return fmt.Errorf("download error: %v", err)
		}
	}

	err = mergeChunks(filename, totalChunks)
	if err != nil {
		return fmt.Errorf("failed to merge chunks: %v", err)
	}

	fmt.Println("11")
	return nil
}

// readDir 读取目录中的文件并发送到channel
func readDir(dir string, urlChan chan string) {
	defer close(urlChan)
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		f, err := os.Open(path.Join(dir, file.Name()))
		if err != nil {
			panic(err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			urlChan <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("Failed to read file: %v\n", err)
		}
	}
}

func main() {
	// 定义请求头
	headers := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Encoding":           "gzip, deflate",
		"Accept-Language":           "zh-CN,zh;q=0.9",
		"Connection":                "keep-alive",
		"Cookie":                    "client_key=F1F17934834AE261DB09A0F0240F342A",
		"DNT":                       "1",
		"Host":                      "down.shuyy8.cc",
		"Referer":                   "http://www.shuyy8.cc/",
		"Upgrade-Insecure-Requests": "1",
		"User-Agent":                "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	}

	// 读取包含URL的文件目录
	FileDir := "task"
	filenames := make(chan string)

	// 启动一个goroutine读取URL文件
	go readDir(FileDir, filenames)

	var wg sync.WaitGroup

	// 启动多个下载线程
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filename := range filenames {
				// 处理文件名
				filename = strings.TrimSpace(filename)
				if filename == "" {
					continue
				}

				url := "http://down.shuyy8.cc/zip/" + filename + ".zip"
				toDir := path.Join("download", filename)
				if err := os.MkdirAll(toDir, 0755); err != nil {
					fmt.Printf("Failed to create directory %s: %v\n", toDir, err)
					continue
				}
				tofile := path.Join(toDir, filename+".zip")

				// 下载文件
				err := downloadFile(url, headers, tofile)
				if err != nil {
					fmt.Printf("Failed to download %s: %v\n", url, err)
				}

				time.Sleep(time.Second)
			}
		}()
	}

	wg.Wait()
	fmt.Println("All downloads completed.")

}
