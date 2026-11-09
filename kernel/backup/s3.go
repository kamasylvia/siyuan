// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type S3Target struct {
	id     string
	client *s3.Client
	bucket string
}

func NewS3Target(target *Target) (*S3Target, error) {
	if target == nil || target.S3 == nil {
		return nil, errors.New("s3 backup target is empty")
	}
	conf := target.S3
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: conf.SkipTLSVerify}
	timeout := time.Duration(conf.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	httpClient := &http.Client{Transport: transport, Timeout: timeout}
	region := conf.Region
	if region == "" {
		region = "us-east-1"
	}
	options := s3.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(conf.AccessKey, conf.SecretKey, ""),
		HTTPClient:   httpClient,
		UsePathStyle: conf.PathStyle,
	}
	if conf.Endpoint != "" {
		options.BaseEndpoint = aws.String(conf.Endpoint)
	}
	return &S3Target{id: target.ID, client: s3.New(options), bucket: conf.Bucket}, nil
}

func (target *S3Target) ID() string { return target.id }

// Fingerprint 返回桶名:S3 桶名全局唯一,同一桶经不同 endpoint 配置时指纹一致
func (target *S3Target) Fingerprint(_ context.Context) (string, error) {
	return "s3/" + target.bucket, nil
}

func s3NotFound(err error) bool {
	var responseErr *smithyhttp.ResponseError
	return errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusNotFound
}

func (target *S3Target) Has(ctx context.Context, key string) (bool, error) {
	_, err := target.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(target.bucket), Key: aws.String(key),
	})
	if err != nil {
		if s3NotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (target *S3Target) Put(ctx context.Context, key string, data []byte) error {
	_, err := target.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(target.bucket), Key: aws.String(key), Body: bytes.NewReader(data),
	})
	return err
}

func (target *S3Target) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := target.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(target.bucket), Key: aws.String(key),
	})
	if err != nil {
		if s3NotFound(err) {
			return nil, errors.New("backup object not found: " + key)
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (target *S3Target) Delete(ctx context.Context, key string) error {
	_, err := target.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(target.bucket), Key: aws.String(key),
	})
	return err
}

func (target *S3Target) List(ctx context.Context, prefix string) ([]string, error) {
	paginator := s3.NewListObjectsV2Paginator(target.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(target.bucket), Prefix: aws.String(prefix),
	})
	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, object := range page.Contents {
			if object.Key != nil {
				keys = append(keys, *object.Key)
			}
		}
	}
	return keys, nil
}
