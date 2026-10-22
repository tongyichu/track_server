package service

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"strconv"
	"strings"
	"time"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

var previewVariants = []string{"square_day", "square_night", "landscape_day", "landscape_night"}

func (s *OSSTokenService) previewBackfillKey(track *models.Track, input PreviewBackfillInput, variant string) string {
	base := strings.Trim(s.ossUploadPrefix, "/")
	if base == "" {
		base = config.DefaultOSSPathPrefix
	}
	return fmt.Sprintf("%s/%d/previews/%d/backfill/%s/%s/%s/%s.jpg", base, GetBucketID(track.UserID), track.UserID, track.ID, input.SourceVersion, input.UploadID, variant)
}

func (s *OSSTokenService) previewPublicEndpoint() string {
	endpoint := strings.TrimRight(s.ossEndpoint, "/")
	if endpoint == "" {
		endpoint = "oss-" + s.ossRegion + ".aliyuncs.com"
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	return endpoint
}

func (s *OSSTokenService) PreviewBackfillUploadURLs(ctx context.Context, track *models.Track, input PreviewBackfillInput) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	credential, err := s.GetUploadCredential(track.UserID)
	if err != nil {
		return nil, err
	}
	client, err := oss.New(s.previewPublicEndpoint(), credential.AccessKeyID, credential.AccessKeySecret, oss.SecurityToken(credential.SecurityToken))
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(s.ossBucket)
	if err != nil {
		return nil, err
	}
	expires := int64(900)
	if until, err := time.Parse(time.RFC3339, credential.Expiration); err == nil {
		if remaining := int64(time.Until(until).Seconds()) - 30; remaining < expires {
			expires = remaining
		}
	}
	if expires <= 0 {
		return nil, fmt.Errorf("upload credential expired")
	}
	urls := make(map[string]string, len(previewVariants))
	for _, variant := range previewVariants {
		urls[variant], err = bucket.SignURL(s.previewBackfillKey(track, input, variant), oss.HTTPPut, expires, oss.ContentType("image/jpeg"))
		if err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func (s *OSSTokenService) VerifyPreviewBackfill(ctx context.Context, track *models.Track, input PreviewBackfillInput) (models.TrackPreviewImages, error) {
	var images models.TrackPreviewImages
	if s.ossInternalEndpoint == "" {
		return images, fmt.Errorf("OSS internal endpoint is required")
	}
	credential, err := s.GetReadCredential(track.UserID)
	if err != nil {
		return images, err
	}
	client, err := oss.New(s.ossInternalEndpoint, credential.AccessKeyID, credential.AccessKeySecret,
		oss.SecurityToken(credential.SecurityToken), oss.Timeout(10, 30))
	if err != nil {
		return images, err
	}
	bucket, err := client.Bucket(s.ossBucket)
	if err != nil {
		return images, err
	}
	urls := make(map[string]string, len(previewVariants))
	for _, variant := range previewVariants {
		key := s.previewBackfillKey(track, input, variant)
		meta, err := bucket.GetObjectDetailedMeta(key, oss.WithContext(ctx))
		if err != nil {
			return images, invalidArg(fmt.Sprintf("preview %s has not been uploaded", variant))
		}
		size, err := strconv.ParseInt(meta.Get("Content-Length"), 10, 64)
		if err != nil || size <= 0 || size > 5*1024*1024 {
			return images, invalidArg("invalid preview image size")
		}
		reader, err := bucket.GetObject(key, oss.Range(0, 262143), oss.WithContext(ctx))
		if err != nil {
			return images, err
		}
		config, format, decodeErr := image.DecodeConfig(io.LimitReader(reader, 262144))
		reader.Close()
		height := 1024
		if strings.HasPrefix(variant, "landscape_") {
			height = 768
		}
		if decodeErr != nil || format != "jpeg" || config.Width != 1024 || config.Height != height {
			return images, invalidArg("invalid preview JPEG dimensions: " + variant)
		}
		host := strings.TrimPrefix(strings.TrimPrefix(s.previewPublicEndpoint(), "https://"), "http://")
		urls[variant] = "https://" + s.ossBucket + "." + host + "/" + key
	}
	images = models.TrackPreviewImages{SquareDayURL: urls["square_day"], SquareNightURL: urls["square_night"],
		LandscapeDayURL: urls["landscape_day"], LandscapeNightURL: urls["landscape_night"]}
	return images, nil
}
