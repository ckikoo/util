package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pierrec/lz4/v4"
)

const (
	defaultImageDir      = "./images"
	defaultCompressedDir = "./compressed_images"
	defaultColdThreshold = 1 * 24 * time.Hour
	checkInterval        = 24 * time.Hour
	cleanUpThreshold     = 7 * 24 * time.Hour
	chunkSize            = 4 * 1024 * 1024 // 4 MB
)

var (
	imageDir      string
	compressedDir string
	coldThreshold time.Duration
	lock          sync.Mutex
)

func init() {
	imageDir = getEnv("IMAGE_DIR", defaultImageDir)
	compressedDir = getEnv("COMPRESSED_DIR", defaultCompressedDir)
	coldThreshold = getEnvDuration("COLD_THRESHOLD", defaultColdThreshold)

	err := os.MkdirAll(imageDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Failed to create image directory: %v\n", err)
		os.Exit(1)
	}

	err = os.MkdirAll(compressedDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Failed to create compressed directory: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/get", getImageHandler).Methods("GET")
	go checkAndCompressColdFiles()

	fmt.Println("Starting server on :8080")
	http.ListenAndServe(":8080", r)
}

func getImageHandler(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("name")
	version := r.URL.Query().Get("version")
	needLatest := r.URL.Query().Get("latest")

	if image == "" {
		http.Error(w, "Please provide image parameter", http.StatusBadRequest)
		return
	}

	if version == "" {
		version = "latest"
	}

	imagePath := getImagePath(sanitizeImageName(image), version)
	compressedPath := getCompressedImagePath(sanitizeImageName(image), version)

	lock.Lock()
	defer lock.Unlock()

	// 如果需要最新镜像，则直接拉取
	if needLatest == "true" {
		if err := pullAndSaveImage(image, version, imagePath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to pull and save image: %v", err), http.StatusInternalServerError)
			return
		}
		serveFileWithCustomName(w, r, imagePath, fmt.Sprintf("%s_%s.tar", sanitizeImageName(image), version))
		return
	}

	// 解压缩文件并返回
	if fileExists(compressedPath) {
		if err := decompressImage(compressedPath, imagePath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to decompress image: %v", err), http.StatusInternalServerError)
			return
		}
		os.Remove(compressedPath)
	}

	// 如果文件不存在，则拉取镜像并保存
	if !fileExists(imagePath) {
		if err := pullAndSaveImage(image, version, imagePath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to pull and save image: %v", err), http.StatusInternalServerError)
			return
		}
	}

	serveFileWithCustomName(w, r, imagePath, fmt.Sprintf("%s_%s.tar", sanitizeImageName(image), version))
}

func checkAndCompressColdFiles() {
	for {
		files, err := os.ReadDir(imageDir)
		fmt.Printf("files: %v\n", files)
		if err != nil {
			fmt.Printf("Failed to read image directory: %v\n", err)
			time.Sleep(checkInterval)
			continue
		}

		for _, file := range files {
			filePath := filepath.Join(imageDir, file.Name())
			if isFileCold(filePath) {
				imageName, version := parseImageAndVersion(file.Name())
				compressedPath := getCompressedImagePath(imageName, version)
				if !fileExists(compressedPath) {
					lock.Lock()
					err := compressImage(filePath, compressedPath)
					lock.Unlock()
					if err != nil {
						fmt.Printf("Failed to compress image: %v\n", err)
					} else {
						os.Remove(filePath) // 删除原始文件
					}
				}
			}
		}

		// 删除超过7天的冷处理文件
		files, err = os.ReadDir(compressedDir)
		if err != nil {
			fmt.Printf("Failed to read compressed directory: %v\n", err)
			time.Sleep(checkInterval)
			continue
		}

		for _, file := range files {
			filePath := filepath.Join(compressedDir, file.Name())
			if isFileExpired(filePath) {
				os.Remove(filePath)
				imageName, version := parseImageAndVersion(file.Name())
				removeImageFromDocker(imageName, version)
				fmt.Printf("Removed expired file and Docker image: %s\n", filePath)
			}
		}

		time.Sleep(checkInterval)
	}
}

func getImagePath(imageName, version string) string {
	return filepath.Join(imageDir, fmt.Sprintf("%s_%s.tar", sanitizeImageName(imageName), version))
}

func getCompressedImagePath(imageName, version string) string {
	return filepath.Join(compressedDir, fmt.Sprintf("%s_%s.lz4", sanitizeImageName(imageName), version))
}

func isFileCold(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	fmt.Printf("info.ModTime(): %v\n", info.ModTime())
	return time.Since(info.ModTime()) > coldThreshold
}

func isFileExpired(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > cleanUpThreshold
}

func parseImageAndVersion(fileName string) (string, string) {
	parts := strings.Split(strings.TrimSuffix(fileName, ".lz4"), "_")
	version := parts[len(parts)-1]
	imageName := strings.Join(parts[:len(parts)-1], "_")
	imageName = strings.ReplaceAll(imageName, "_", "/")
	return imageName, version
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

func pullAndSaveImage(image, version, imagePath string) error {
	fullImageName := fmt.Sprintf("%s:%s", image, version)
	cmd := exec.Command("docker", "pull", fullImageName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to pull image: %s, output: %s", err, output)
	}

	cmd = exec.Command("docker", "save", "-o", imagePath, fullImageName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to save image: %s, output: %s", err, output)
	}

	return nil
}

func removeImageFromDocker(image, version string) error {
	fullImageName := fmt.Sprintf("%s:%s", image, version)
	cmd := exec.Command("docker", "rmi", fullImageName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove image: %s, output: %s", err, output)
	}

	return nil
}

func compressImage(srcPath, destPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	writer := lz4.NewWriter(destFile)
	defer writer.Close()

	buf := make([]byte, chunkSize)
	for {
		n, err := srcFile.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		if _, err := writer.Write(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

func decompressImage(srcPath, destPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	reader := lz4.NewReader(srcFile)

	buf := make([]byte, chunkSize)
	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		if _, err := destFile.Write(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

func serveFileWithCustomName(w http.ResponseWriter, r *http.Request, filePath, fileName string) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filePath)
}

func getEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	return value
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func sanitizeImageName(imageName string) string {
	return strings.ReplaceAll(imageName, "/", "_")
}
