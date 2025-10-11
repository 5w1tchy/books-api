package s3

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Client struct {
	Client    *s3.Client
	Presigner *s3.PresignClient
	Bucket    string
}

// NewR2Client initializes an S3-compatible client for Cloudflare R2
func NewR2Client(ctx context.Context) (*S3Client, error) {
	endpoint := os.Getenv("AWS_ENDPOINT")
	region := os.Getenv("AWS_REGION")
	bucket := os.Getenv("AWS_BUCKET")

	creds := credentials.NewStaticCredentialsProvider(
		os.Getenv("AWS_ACCESS_KEY_ID"),
		os.Getenv("AWS_SECRET_ACCESS_KEY"),
		"",
	)

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = false
	})

	presigner := s3.NewPresignClient(client)

	return &S3Client{
		Client:    client,
		Presigner: presigner,
		Bucket:    bucket,
	}, nil
}

// GeneratePresignedUploadURL creates a presigned PUT URL for direct upload
func (s *S3Client) GeneratePresignedUploadURL(ctx context.Context, objectKey, contentType string) (string, error) {
	req, err := s.Presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 15 * time.Minute // URL valid for 15 minutes
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign upload: %w", err)
	}
	return req.URL, nil
}

// GeneratePresignedDownloadURL creates a presigned GET URL for downloading/streaming
func (s *S3Client) GeneratePresignedDownloadURL(ctx context.Context, objectKey string) (string, error) {
	req, err := s.Presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(objectKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 15 * time.Minute // URL valid for 15 minutes
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign download: %w", err)
	}
	return req.URL, nil
}
