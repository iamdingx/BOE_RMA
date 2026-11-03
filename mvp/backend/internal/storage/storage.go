// Package storage 对象存储抽象（生产 MinIO，开发/测试内存实现）。
package storage

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage 对象存储接口。
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (url string, err error)
	URL(key string) string
}

// ---------- MinIO ----------

type MinIO struct {
	cli    *minio.Client
	bucket string
	public string
}

func NewMinIO(endpoint, accessKey, secretKey, bucket string, useSSL bool, publicBase string) (*MinIO, error) {
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &MinIO{cli: cli, bucket: bucket, public: publicBase}, nil
}

func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := m.cli.PutObject(ctx, m.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", err
	}
	return m.URL(key), nil
}

func (m *MinIO) URL(key string) string {
	if m.public != "" {
		return fmt.Sprintf("%s/%s/%s", m.public, m.bucket, key)
	}
	return fmt.Sprintf("/objects/%s/%s", m.bucket, key)
}

// ---------- 内存实现（开发/测试） ----------

type Mem struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func NewMem() *Mem { return &Mem{blobs: map[string][]byte{}} }

func (m *Mem) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.blobs[key] = b
	m.mu.Unlock()
	return m.URL(key), nil
}

func (m *Mem) URL(key string) string { return "/objects/mem/" + key }

func (m *Mem) Get(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blobs[key]
	return b, ok
}
