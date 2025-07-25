package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOClient struct {
	client *minio.Client
}

func NewMinIOClient(endpoint, accessKey, secretKey string, useSSL bool) (*MinIOClient, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}
	return &MinIOClient{client: minioClient}, nil
}

func (c *MinIOClient) Upload(ctx context.Context, bucketName, objectName, contentType string, reader io.Reader, size int64) (minio.UploadInfo, error) {
	info, err := c.client.PutObject(ctx, bucketName, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return minio.UploadInfo{}, fmt.Errorf("failed to upload object: %w", err)
	}
	return info, nil
}
