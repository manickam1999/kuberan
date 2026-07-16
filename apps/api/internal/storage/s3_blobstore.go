package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

// S3Config configures an S3BlobStore. For MinIO, set Endpoint to the internal
// URL (e.g. http://minio:9000) and UsePathStyle to true.
type S3Config struct {
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	Region       string // optional; MinIO ignores it, default "us-east-1"
	UsePathStyle bool
}

// S3BlobStore is a BlobStore backed by any S3-compatible object store.
type S3BlobStore struct {
	client *s3.Client
	bucket string
}

// NewS3BlobStore builds an S3BlobStore from cfg using static credentials.
func NewS3BlobStore(cfg S3Config) (*S3BlobStore, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("storage: bucket is required")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: cfg.UsePathStyle,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		),
	})

	return &S3BlobStore{client: client, bucket: cfg.Bucket}, nil
}

// Put uploads the bytes read from r to key.
func (s *S3BlobStore) Put(ctx context.Context, key string, r io.Reader, contentType string, size int64) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("storage: put %q: %w", key, err)
	}
	return nil
}

// Get returns a reader for the object at key, or ErrNotFound.
func (s *S3BlobStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	return out.Body, nil
}

// Delete removes the object at key. Deleting a missing key is not an error.
func (s *S3BlobStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}

// isS3NotFound reports whether err is an S3 "no such key" / 404 response.
func isS3NotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
