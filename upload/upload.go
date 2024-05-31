package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// ReadDir 读取目录中的所有文件路径，并将其发送到通道
func ReadDir(dir string, pathChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			pathChan <- path
		}
		return nil
	})
	close(pathChan)
}

// Upload 从通道中读取文件路径并上传到 S3
func Upload(pathChan <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()

	accessKeyID := os.Getenv("accessKeyID")
	secretAccessKey := os.Getenv("secretAccessKey")
	region := "cn-sy1" // 根据自定义终端的区域设置
	endpoint := "https://cn-sy1.rains3.com"
	bucket := "noval"

	// 创建一个新的会话
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Credentials:      credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		Endpoint:         aws.String(endpoint), // 指定自定义终端
		S3ForcePathStyle: aws.Bool(true),       // 强制使用路径样式
	})

	if err != nil {
		fmt.Println("创建会话失败", err)
		return
	}

	// 创建 S3 服务客户端
	svc := s3.New(sess)

	for filePath := range pathChan {
		key := filepath.Base(filePath)

		// 打开文件
		file, err := os.Open(filePath)
		if err != nil {
			fmt.Println("无法打开文件", err)
			continue
		}

		// 上传文件
		_, err = svc.PutObject(&s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   file,
		})

		file.Close()

		if err != nil {
			fmt.Println("上传失败", err)
			continue
		}

		fmt.Println("文件上传成功:", filePath)
	}
}

func main() {
	var wg sync.WaitGroup
	pathChan := make(chan string, 10)

	// 启动 ReadDir Goroutine
	wg.Add(1)
	go ReadDir("dir", pathChan, &wg)

	// 启动多个 Upload Goroutine 以并发上传文件
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go Upload(pathChan, &wg)
	}

	// 等待所有 Goroutine 完成
	wg.Wait()
}
