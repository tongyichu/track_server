package service

import (
	"context"
	"errors"
	"fmt"
	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/tongyichu/track_server/internal/models"
	"image"
	_ "image/jpeg"
	"io"
	"os"
	"strconv"
	"strings"
)

func (s *OSSTokenService) CommentUpload(ctx context.Context, user int64, id string) (*CommentUpload, error) {
	credential, err := s.GetUploadCredential(user)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSuffix(credential.Dir, "/") + fmt.Sprintf("/comments/%d/%s.jpg", user, id)
	host := strings.TrimPrefix(strings.TrimPrefix(credential.Endpoint, "https://"), "http://")
	return &CommentUpload{ObjectKey: key, OSSURL: "https://" + credential.Bucket + "." + strings.TrimSuffix(host, "/") + "/" + key, Credential: credential}, nil
}
func (s *OSSTokenService) VerifyCommentImage(ctx context.Context, imageInfo *models.CommentImage) error {
	if s.ossInternalEndpoint == "" {
		return errors.New("OSS internal endpoint is required for comment images")
	}
	credential, err := s.GetReadCredential(imageInfo.UserID)
	if err != nil {
		return err
	}
	client, err := oss.New(s.ossInternalEndpoint, credential.AccessKeyID, credential.AccessKeySecret, oss.SecurityToken(credential.SecurityToken))
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(s.ossBucket)
	if err != nil {
		return err
	}
	meta, err := bucket.GetObjectDetailedMeta(imageInfo.ObjectKey, oss.WithContext(ctx))
	if err != nil {
		return invalidArg("comment image has not been uploaded")
	}
	size, err := strconv.ParseInt(meta.Get("Content-Length"), 10, 64)
	if err != nil || size != imageInfo.ByteSize || size > 5*1024*1024 {
		return invalidArg("invalid comment image size")
	}
	reader, err := bucket.GetObject(imageInfo.ObjectKey, oss.Range(0, 262143), oss.WithContext(ctx))
	if err != nil {
		return err
	}
	defer reader.Close()
	config, format, err := image.DecodeConfig(io.LimitReader(reader, 262144))
	if err != nil || format != "jpeg" || config.Width != imageInfo.Width || config.Height != imageInfo.Height {
		return invalidArg("invalid JPEG comment image")
	}
	return nil
}

// Comment image downloads stay bounded even if an uploaded object is subsequently replaced.
func (s *OSSTokenService) DownloadCommentImage(user int64, sourceURL, destination string) error {
	bucketName, _, key, err := s.parseOSSObjectURL(sourceURL)
	if err != nil {
		return err
	}
	if bucketName != s.ossBucket || s.ossInternalEndpoint == "" {
		return errors.New("invalid comment image storage")
	}
	credential, err := s.GetReadCredential(user)
	if err != nil {
		return err
	}
	client, err := oss.New(s.ossInternalEndpoint, credential.AccessKeyID, credential.AccessKeySecret, oss.SecurityToken(credential.SecurityToken), oss.Timeout(10, 30))
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return err
	}
	reader, err := bucket.GetObject(key)
	if err != nil {
		return err
	}
	defer reader.Close()
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(file, io.LimitReader(reader, 5*1024*1024+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > 5*1024*1024 {
		return invalidArg("comment image exceeds size limit")
	}
	check, err := os.Open(destination)
	if err != nil {
		return err
	}
	defer check.Close()
	config, format, err := image.DecodeConfig(check)
	if err != nil || format != "jpeg" || config.Width > 4096 || config.Height > 4096 {
		return invalidArg("invalid comment image")
	}
	return nil
}

type commentImageDownloader struct{ objects CommentObjectStore }

func (d commentImageDownloader) DownloadObject(user int64, source, destination string) error {
	return d.objects.DownloadCommentImage(user, source, destination)
}
