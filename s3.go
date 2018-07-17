package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func ListBuckets() []*S3Object {
	client := getS3Client()
	ctx := context.Background()
	timeout := time.Second * 30
	var cancelFn func()
	if timeout > 0 {
		ctx, cancelFn = context.WithTimeout(ctx, timeout)
	}
	defer cancelFn()

	result, err := client.ListBucketsWithContext(ctx, &s3.ListBucketsInput{})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			log.Printf("upload canceled due to timeout, %v", err)
		} else {
			log.Printf("failed to upload object, %v", err)
		}
		os.Exit(1)
	}

	var objects []*S3Object
	for _, bucket := range result.Buckets {
		obj := NewS3Object(
			Bucket,
			aws.StringValue(bucket.Name),
			bucket.CreationDate,
			nil,
		)
		objects = append(objects, obj)
	}
	return objects
}

func ListObjects(bucket, prefix string) []*S3Object {
	client := getS3Client()
	ctx := context.Background()
	timeout := time.Second * 30
	var cancelFn func()
	if timeout > 0 {
		ctx, cancelFn = context.WithTimeout(ctx, timeout)
	}
	defer cancelFn()

	result, err := client.ListObjectsWithContext(ctx, &s3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
		Prefix:    aws.String(prefix),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			log.Printf("upload canceled due to timeout, %v", err)
		} else {
			log.Printf("failed to upload object, %v", err)
		}
		os.Exit(1)
	}

	var objects []*S3Object

	obj := NewS3Object(
		PreDir,
		"..",
		nil,
		nil,
	)
	objects = append(objects, obj)

	for _, commonPrefix := range result.CommonPrefixes {
		obj := NewS3Object(
			Dir,
			aws.StringValue(commonPrefix.Prefix),
			nil,
			nil,
		)
		objects = append(objects, obj)
	}
	for _, content := range result.Contents {
		obj := NewS3Object(
			Object,
			aws.StringValue(content.Key),
			content.LastModified,
			content.Size,
		)
		objects = append(objects, obj)
	}
	return objects
}

func DownloadObject(bucket, key string) {
	client := getS3Downloader()
	ctx := context.Background()
	timeout := time.Second * 30
	var cancelFn func()
	if timeout > 0 {
		ctx, cancelFn = context.WithTimeout(ctx, timeout)
	}
	defer cancelFn()

	currentDir, _ := os.Getwd()
	f, err := os.Create(filepath.Join(currentDir, key))
	if err != nil {
		log.Fatalf("donwload file create failed, %v", err)
	}
	defer f.Close()

	n, err := client.DownloadWithContext(ctx, f, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == request.CanceledErrorCode {
			log.Printf("upload canceled due to timeout, %v", err)
		} else {
			log.Printf("failed to upload object, %v", err)
		}
		os.Exit(1)
	}
	log.Printf("download success, %v", n)
}

func getS3Downloader() *s3manager.Downloader {
	var sess *session.Session
	if mockFlag {
		sess = getMinioSession()
	} else {
		sess = getAWSSession()
	}
	return s3manager.NewDownloader(sess)
}

func getS3Client() *s3.S3 {
	var sess *session.Session
	if mockFlag {
		sess = getMinioSession()
	} else {
		sess = getAWSSession()
	}
	return s3.New(sess)
}

func getMinioSession() *session.Session {
	cfg := &aws.Config{
		Credentials:      credentials.NewStaticCredentials("access_key", "secret_key", ""),
		Endpoint:         aws.String("localhost:9000"),
		Region:           aws.String("ap-northeast-1"),
		DisableSSL:       aws.Bool(true),
		S3ForcePathStyle: aws.Bool(true),
	}
	return session.New(cfg)
}

func getAWSSession() *session.Session {
	return session.Must(session.NewSession())
}
