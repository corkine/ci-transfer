package transfer

import (
	"context"
	"fmt"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

func newOSSClient(c ossConfig, a Options) *oss.Client {
	cfg := oss.LoadDefaultConfig().
		WithEndpoint(c.Endpoint).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.KeyID, c.KeySecret)).
		WithRetryMaxAttempts(1). // A single retry loop controls exact attempt counts and logs.
		WithConnectTimeout(a.ConnectTimeout).
		WithReadWriteTimeout(a.Timeout)
	if c.Region != "" {
		cfg.WithRegion(c.Region)
	} else {
		// Existing configurations for custom/acceleration endpoints have no region.
		// Preserve their V1 authentication; oss_region opts them into V4.
		cfg.WithSignatureVersion(oss.SignatureVersionV1)
	}
	return oss.NewClient(cfg)
}

func handleOSS(ctx context.Context, a Options) error {
	c, err := parseOSS(a.OSS)
	if err != nil {
		return err
	}
	plan, err := planTransfers(a.Sources, c.Path, true)
	if err != nil {
		return err
	}
	client := newOSSClient(c, a)
	for _, file := range plan.Files {
		start := time.Now()
		fmt.Printf("oss transfer: %s (%d bytes)\n", file.Remote, file.Size)
		err := withRetry(ctx, a, "OSS upload "+file.Remote, func(ctx context.Context) error {
			attempt, cancel := context.WithTimeout(ctx, a.Timeout)
			defer cancel()
			_, err := client.PutObjectFromFile(attempt, &oss.PutObjectRequest{Bucket: oss.Ptr(c.Bucket), Key: oss.Ptr(file.Remote)}, file.Local)
			return err
		})
		if err != nil {
			return err
		}
		fmt.Printf("Uploaded: %s (%d bytes, %s)\n", file.Remote, file.Size, time.Since(start).Round(time.Millisecond))
	}
	if len(a.Sources) == 0 {
		fmt.Println("OSS configuration valid; no source files specified")
	}
	return nil
}
