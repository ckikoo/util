package main

import (
	"fmt"
	"io/ioutil"
	"jiaoben-/upload/model"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testBucket    = "noval"
	testRegion    = "cn-sy1"
	testEndpoint  = "https://cn-sy1.rains3.com" // 本地模拟S3的端点（如使用LocalStack）
	testAccessKey = ""
	testSecretKey = ""
)

// 初始化 S3 客户端
func newTestS3Client(t *testing.T) *model.S3Client {
	client, err := model.NewS3Client(testAccessKey, testSecretKey, testRegion, testEndpoint, testBucket)
	assert.NoError(t, err)
	return client
}

// 创建测试文件
func createTestFile(t *testing.T, name string, content []byte) {
	err := ioutil.WriteFile(name, content, 0644)
	assert.NoError(t, err)
}

// 删除测试文件
func deleteTestFile(name string) {
	os.Remove(name)
}

// TestUploadFile 测试上传文件
func TestUploadFile(t *testing.T) {
	client := newTestS3Client(t)

	testFileName := "test_upload_file.txt"
	testFileContent := []byte("This is a test file content")
	createTestFile(t, testFileName, testFileContent)
	defer deleteTestFile(testFileName)

	err := client.UploadFile(testFileName, 5*1024*1024) // 5MB 分片大小
	assert.NoError(t, err)

	// 验证文件是否上传成功
	_, err = client.GetFileInfo(testFileName)
	assert.NoError(t, err)
}

// TestMultipartUploadFile 测试分片上传文件
func TestMultipartUploadFile(t *testing.T) {
	client := newTestS3Client(t)

	testFileName := "test_multipart_upload_file.txt"
	testFileContent := make([]byte, 10*1024*1024) // 10MB 文件
	createTestFile(t, testFileName, testFileContent)
	defer deleteTestFile(testFileName)

	err := client.MultipartUploadFile(testFileName, 5*1024*1024) // 5MB 分片大小
	assert.NoError(t, err)

	// 验证文件是否上传成功
	_, err = client.GetFileInfo(testFileName)
	assert.NoError(t, err)
}

// TestDownloadFile 测试下载文件
func TestDownloadFile(t *testing.T) {
	client := newTestS3Client(t)

	testFileName := "test_download_file.txt"
	testFileContent := []byte("This is a test file content")
	createTestFile(t, testFileName, testFileContent)
	defer deleteTestFile(testFileName)

	err := client.UploadFile(testFileName, 5*1024*1024) // 5MB 分片大小
	assert.NoError(t, err)

	downloadFileName := "downloaded_test_file.txt"
	defer deleteTestFile(downloadFileName)

	err = client.DownloadFile(testFileName, downloadFileName)
	assert.NoError(t, err)

	downloadedContent, err := ioutil.ReadFile(downloadFileName)
	assert.NoError(t, err)
	assert.Equal(t, testFileContent, downloadedContent)
}

// TestDeleteFile 测试删除文件
func TestDeleteFile(t *testing.T) {
	client := newTestS3Client(t)

	testFileName := "test_delete_file.txt"
	testFileContent := []byte("This is a test file content")
	createTestFile(t, testFileName, testFileContent)
	defer deleteTestFile(testFileName)

	err := client.UploadFile(testFileName, 5*1024*1024) // 5MB 分片大小
	assert.NoError(t, err)

	err = client.DeleteFile(testFileName)
	assert.NoError(t, err)

	// 验证文件是否删除成功
	_, err = client.GetFileInfo(testFileName)
	fmt.Printf("err: %v\n", err)
	assert.Error(t, err)
}

// TestListFiles 测试列出文件
func TestListFiles(t *testing.T) {
	client := newTestS3Client(t)

	files, err := client.ListFiles("13", 2)
	assert.NoError(t, err)
	fmt.Printf("files: %v\n", files)
	fmt.Printf("len(files): %v\n", len(files))
}

// TestGetFileInfo 测试获取文件信息
func TestGetFileInfo(t *testing.T) {
	client := newTestS3Client(t)

	// testFileName := "test_get_file_info.txt"
	// testFileContent := []byte("This is a test file content")
	// createTestFile(t, testFileName, testFileContent)
	// defer deleteTestFile(testFileName)

	// err := client.UploadFile(testFileName, 5*1024*1024) // 5MB 分片大小
	// assert.NoError(t, err)

	info, err := client.GetFileInfo("2013.txt")
	fmt.Printf("info: %+v\n", info)
	assert.NoError(t, err)
	assert.NotNil(t, info)
}

func main() {
	// Running the tests
	fmt.Println("Running the tests...")
	t := &testing.T{}
	// TestUploadFile(t)
	// TestMultipartUploadFile(t)
	// TestDownloadFile(t)
	// TestDeleteFile(t)
	TestListFiles(t)
	TestGetFileInfo(t)
}
