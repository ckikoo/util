package model

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// S3Client encapsulates the S3 client and its operations
type S3Client struct {
	svc    *s3.S3
	bucket string
}

// NewS3Client creates a new S3Client instance
func NewS3Client(accessKeyID, secretAccessKey, region, endpoint, bucket string) (*S3Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Credentials:      credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	return &S3Client{
		svc:    s3.New(sess),
		bucket: bucket,
	}, nil
}

// UploadFile chooses between simple upload or multipart upload based on file size
func (client *S3Client) UploadFile(filePath string, partSize int64) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	if fileInfo.Size() > partSize {
		return client.MultipartUploadFile(filePath, partSize)
	}
	return client.SimpleUploadFile(filePath)
}

// SimpleUploadFile uploads a file to S3 using simple upload
func (client *S3Client) SimpleUploadFile(filePath string) error {
	key := filepath.Base(filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	uploader := s3manager.NewUploaderWithClient(client.svc)

	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("upload failed: %v", err)
	}

	fmt.Println("file uploaded successfully:", filePath)
	return nil
}

// MultipartUploadFile uploads a file to S3 using multipart upload
func (client *S3Client) MultipartUploadFile(filePath string, partSize int64) error {
	key := filepath.Base(filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	uploadID, err := client.InitMultipartUpload(key)
	if err != nil {
		return err
	}

	completedParts, err := client.UploadParts(file, key, uploadID, partSize)
	if err != nil {
		client.AbortMultipartUpload(&key, uploadID)
		return err
	}

	err = client.CompleteMultipartUpload(key, uploadID, completedParts)
	if err != nil {
		return err
	}

	fmt.Println("file uploaded successfully (multipart):", filePath)
	return nil
}

// InitMultipartUpload initializes a multipart upload
func (client *S3Client) InitMultipartUpload(key string) (*string, error) {
	createResp, err := client.svc.CreateMultipartUpload(&s3.CreateMultipartUploadInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initiate multipart upload: %v", err)
	}
	return createResp.UploadId, nil
}

// UploadParts uploads parts of a file in a multipart upload
func (client *S3Client) UploadParts(file *os.File, key string, uploadID *string, partSize int64) ([]*s3.CompletedPart, error) {
	var completedParts []*s3.CompletedPart
	buffer := make([]byte, partSize)
	partNumber := int64(1)

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read file: %v", err)
		}
		if n == 0 {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		uploadResp, err := client.UploadPartWithRetry(ctx, buffer[:n], key, uploadID, partNumber, 3)
		if err != nil {
			return nil, err
		}

		completedParts = append(completedParts, &s3.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int64(partNumber),
		})
		partNumber++
	}

	return completedParts, nil
}

// CompleteMultipartUpload completes a multipart upload
func (client *S3Client) CompleteMultipartUpload(key string, uploadID *string, completedParts []*s3.CompletedPart) error {
	_, err := client.svc.CompleteMultipartUpload(&s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(client.bucket),
		Key:      aws.String(key),
		UploadId: uploadID,
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %v", err)
	}
	return nil
}

// UploadPartWithRetry uploads a single part with retry logic
func (client *S3Client) UploadPartWithRetry(ctx context.Context, buffer []byte, key string, uploadID *string, partNumber int64, retries int) (*s3.UploadPartOutput, error) {
	var uploadResp *s3.UploadPartOutput
	var err error

	for i := 0; i < retries; i++ {
		uploadResp, err = client.svc.UploadPartWithContext(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(client.bucket),
			Key:        aws.String(key),
			PartNumber: aws.Int64(partNumber),
			UploadId:   uploadID,
			Body:       bytes.NewReader(buffer),
		})
		if err == nil {
			return uploadResp, nil
		}
		fmt.Println("failed to upload part, retrying:", err)
		time.Sleep(2 * time.Second)
	}
	return nil, err
}

// AbortMultipartUpload aborts a multipart upload
func (client *S3Client) AbortMultipartUpload(key, uploadID *string) {
	_, err := client.svc.AbortMultipartUpload(&s3.AbortMultipartUploadInput{
		Bucket:   aws.String(client.bucket),
		Key:      key,
		UploadId: uploadID,
	})
	if err != nil {
		fmt.Println("failed to abort multipart upload:", err)
	}
}

// DownloadFile downloads a file from S3 to the local filesystem
func (client *S3Client) DownloadFile(key, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	resp, err := client.svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read file content: %v", err)
	}

	fmt.Println("file downloaded successfully:", filePath)
	return nil
}

// ListFiles lists files in the S3 bucket with optional filtering and pagination
func (client *S3Client) ListFiles(filter string, lmit int64) ([]string, error) {
	var fileList []string
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(client.bucket),
			ContinuationToken: continuationToken,
		}

		resp, err := client.svc.ListObjectsV2(input)
		if err != nil {
			return nil, fmt.Errorf("failed to list files: %v", err)
		}

		for _, item := range resp.Contents {
			if filter == "" || strings.Contains(*item.Key, filter) {
				fileList = append(fileList, *item.Key)
				if len(fileList) >= int(lmit) && lmit != 0 {
					return fileList, nil
				}
			}
		}

		if !aws.BoolValue(resp.IsTruncated) {
			break
		}

		continuationToken = resp.NextContinuationToken
	}

	return fileList, nil
}

// DeleteFile deletes a file from the S3 bucket
func (client *S3Client) DeleteFile(key string) error {
	_, err := client.svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete file: %v", err)
	}

	fmt.Println("file deleted successfully:", key)
	return nil
}

// GetFileInfo retrieves information about a file in the S3 bucket
func (client *S3Client) GetFileInfo(key string) (*s3.HeadObjectOutput, error) {
	resp, err := client.svc.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}
	return resp, nil
}
