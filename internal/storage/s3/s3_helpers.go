package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DeleteObject deletes an object from the bucket (used for cleanup).
func (s *S3Client) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("s3: delete object %s: %w", objectKey, err)
	}
	return nil
}
