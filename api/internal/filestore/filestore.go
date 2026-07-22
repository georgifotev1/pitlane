// Package filestore provides object storage behind a small interface. The
// implementation uses the AWS SDK v2 S3 client, which works with both
// Cloudflare R2 in production and MinIO in local development (ADR decision 15).
package filestore

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Object is the minimal metadata returned from a Get.
type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// Store is the object-storage interface. Callers are responsible for
// tenant-prefixing keys; the implementation stores whatever key it is given.
type Store interface {
	// Put writes an object. The caller provides the full key and content type.
	Put(ctx context.Context, key, contentType string, body io.Reader) error
	// Get returns the object body and metadata. The caller must close Body.
	Get(ctx context.Context, key string) (*Object, error)
	// Delete removes an object. It is idempotent: missing objects do not error.
	Delete(ctx context.Context, key string) error
}

// Config is the runtime configuration for the S3-compatible backend.
type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

// S3Store is a Store backed by AWS SDK v2 S3. It works with R2, MinIO, and any
// other S3-compatible service by overriding the endpoint and using path-style
// addressing (required for MinIO, acceptable for R2).
type S3Store struct {
	client *s3.Client
	bucket string
}

// NewS3 builds an S3-backed Store. It does not verify the bucket; the first
// operation will surface any credential/endpoint issue.
func NewS3(cfg Config) (Store, error) {
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		}
	})

	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

// Put uploads an object. It streams body directly to S3 without buffering the
// whole file in memory.
func (s *S3Store) Put(ctx context.Context, key, contentType string, body io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

// Get fetches an object. The caller is responsible for closing Body.
func (s *S3Store) Get(ctx context.Context, key string) (*Object, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}

	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}

	return &Object{
		Body:        out.Body,
		ContentType: contentType,
		Size:        size,
	}, nil
}

// Delete removes an object. Missing objects are not treated as errors so callers
// can use it for cleanup without existence checks.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
