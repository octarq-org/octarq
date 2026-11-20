package db

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/octarq-org/octarq/server/config"
)

type BackupObject struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

type S3Uploader struct {
	client *s3.Client
	bucket string
	secret string
	keyID  string
}

func NewS3Uploader(cfg *config.Config, optFns ...func(*s3.Options)) (*S3Uploader, error) {
	if cfg == nil {
		return nil, errors.New("s3: nil configuration")
	}
	if strings.TrimSpace(cfg.BackupS3Bucket) == "" {
		return nil, errors.New("s3: OCTARQ_BACKUP_S3_BUCKET is required")
	}
	if strings.TrimSpace(cfg.BackupS3AccessKey) == "" {
		return nil, errors.New("s3: OCTARQ_BACKUP_S3_ACCESS_KEY is required")
	}
	if strings.TrimSpace(cfg.BackupS3SecretKey) == "" {
		return nil, errors.New("s3: OCTARQ_BACKUP_S3_SECRET_KEY is required")
	}

	region := strings.TrimSpace(cfg.BackupS3Region)
	if region == "" {
		if strings.TrimSpace(cfg.BackupS3Endpoint) == "" {
			region = "us-east-1"
		} else {
			region = "auto"
		}
	}

	opts := s3.Options{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.BackupS3AccessKey,
			cfg.BackupS3SecretKey,
			"",
		),
	}

	if endpoint := strings.TrimSpace(cfg.BackupS3Endpoint); endpoint != "" {
		opts.BaseEndpoint = aws.String(endpoint)
		opts.UsePathStyle = true
	}

	for _, fn := range optFns {
		if fn != nil {
			fn(&opts)
		}
	}

	client := s3.New(opts)
	return &S3Uploader{
		client: client,
		bucket: strings.TrimSpace(cfg.BackupS3Bucket),
		secret: cfg.BackupS3SecretKey,
		keyID:  cfg.BackupS3AccessKey,
	}, nil
}

func (u *S3Uploader) Upload(ctx context.Context, localPath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("s3: read local backup %q: %w", localPath, err)
	}

	remoteKey := filepath.Base(localPath)
	localMD5 := md5.Sum(data)
	localMD5Hex := hex.EncodeToString(localMD5[:])

	localSHA256 := sha256.Sum256(data)
	localSHA256Hex := hex.EncodeToString(localSHA256[:])

	input := &s3.PutObjectInput{
		Bucket:        aws.String(u.bucket),
		Key:           aws.String(remoteKey),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}

	output, err := u.client.PutObject(ctx, input)
	if err != nil {
		return u.sanitize(fmt.Errorf("s3: put object %q to bucket %q: %w", remoteKey, u.bucket, err))
	}

	if output.ETag != nil {
		remoteETag := strings.Trim(aws.ToString(output.ETag), "\"")
		if !strings.Contains(remoteETag, "-") && !strings.EqualFold(remoteETag, localMD5Hex) {
			return fmt.Errorf("s3: backup checksum mismatch: remote ETag %s != local MD5 %s", remoteETag, localMD5Hex)
		}
	}
	if output.ChecksumSHA256 != nil {
		remoteSHA256 := aws.ToString(output.ChecksumSHA256)
		if remoteSHA256 != "" && !strings.EqualFold(remoteSHA256, localSHA256Hex) {
			return fmt.Errorf("s3: backup checksum mismatch: remote SHA256 %s != local SHA256 %s", remoteSHA256, localSHA256Hex)
		}
	}

	return nil
}

func (u *S3Uploader) Download(ctx context.Context, remoteKey, localPath string) error {
	input := &s3.GetObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(remoteKey),
	}

	output, err := u.client.GetObject(ctx, input)
	if err != nil {
		return u.sanitize(fmt.Errorf("s3: get object %q from bucket %q: %w", remoteKey, u.bucket, err))
	}
	defer output.Body.Close()

	if dir := filepath.Dir(localPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("s3: create local directory: %w", err)
		}
	}

	tmpFile := localPath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("s3: create temp download file: %w", err)
	}

	if _, err := io.Copy(f, output.Body); err != nil {
		f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("s3: write download stream: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("s3: sync download file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("s3: close download file: %w", err)
	}

	if err := os.Rename(tmpFile, localPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("s3: finalize downloaded backup: %w", err)
	}

	return nil
}

func (u *S3Uploader) ListBackups(ctx context.Context) ([]BackupObject, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(u.bucket),
		Prefix: aws.String("octarq-backup-"),
	}

	output, err := u.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, u.sanitize(fmt.Errorf("s3: list objects in bucket %q: %w", u.bucket, err))
	}

	var results []BackupObject
	for _, obj := range output.Contents {
		key := aws.ToString(obj.Key)
		var lastMod time.Time
		if obj.LastModified != nil {
			lastMod = *obj.LastModified
		}
		results = append(results, BackupObject{
			Key:          key,
			Size:         aws.ToInt64(obj.Size),
			LastModified: lastMod,
			ETag:         aws.ToString(obj.ETag),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].LastModified.After(results[j].LastModified)
	})

	return results, nil
}

func (u *S3Uploader) LatestBackup(ctx context.Context) (*BackupObject, error) {
	backups, err := u.ListBackups(ctx)
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, fmt.Errorf("s3: no backup objects found in bucket %q", u.bucket)
	}
	return &backups[0], nil
}

func (u *S3Uploader) LatestBackupKey(ctx context.Context) (string, error) {
	obj, err := u.LatestBackup(ctx)
	if err != nil {
		return "", err
	}
	return obj.Key, nil
}

func (u *S3Uploader) PruneOlderThan(ctx context.Context, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	backups, err := u.ListBackups(ctx)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, b := range backups {
		if b.LastModified.Before(cutoff) {
			delInput := &s3.DeleteObjectInput{
				Bucket: aws.String(u.bucket),
				Key:    aws.String(b.Key),
			}
			if _, err := u.client.DeleteObject(ctx, delInput); err != nil {
				return deleted, u.sanitize(fmt.Errorf("s3: delete object %q: %w", b.Key, err))
			}
			deleted++
		}
	}

	return deleted, nil
}

func (u *S3Uploader) sanitize(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if u.secret != "" {
		msg = strings.ReplaceAll(msg, u.secret, "[REDACTED_SECRET]")
	}
	if u.keyID != "" && len(u.keyID) > 4 {
		msg = strings.ReplaceAll(msg, u.keyID, "[REDACTED_KEY]")
	}
	return errors.New(msg)
}
