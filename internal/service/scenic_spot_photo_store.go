package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	_ "golang.org/x/image/webp"
	"golang.org/x/sync/singleflight"
)

const scenicPhotoMaxBytes = 8 << 20

// Only explicit catalog writes/backfills use this store; reads never fetch media.
type ScenicSpotPhotoStore struct {
	root   string
	client *http.Client
	slots  chan struct{}
	flight singleflight.Group
}

func NewScenicSpotPhotoStore(staticRoot string) *ScenicSpotPhotoStore {
	return &ScenicSpotPhotoStore{
		root: staticRoot, slots: make(chan struct{}, 2),
		client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !allowedScenicPhotoURL(req.URL.String()) {
				return errors.New("景点图片重定向地址无效")
			}
			return nil
		}},
	}
}

func allowedScenicPhotoURL(raw string) bool {
	canonical := safeAMapPhotoURL(raw)
	if canonical == "" || !strings.HasPrefix(raw, "https://") {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, canonical, nil)
	return err == nil && (req.URL.Port() == "" || req.URL.Port() == "443")
}

func scenicPhotoCachePath(source string) string {
	// Include the encoding policy in the key so future policy changes cannot reuse old files.
	sum := sha256.Sum256([]byte("webp-q75-landscape1024-portrait720-v1\n" + source))
	return filepath.Join("scenic_spots", "amap", hex.EncodeToString(sum[:])+".webp")
}

func scenicPhotoCachedURL(source string) string {
	return "/api/v1/static/" + filepath.ToSlash(scenicPhotoCachePath(source))
}

func validScenicPhotoFile(name string) bool {
	file, err := os.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > scenicPhotoMaxBytes {
		return false
	}
	config, format, err := image.DecodeConfig(io.LimitReader(file, scenicPhotoMaxBytes))
	return err == nil && format == "webp" && (config.Width >= config.Height && config.Width == 1024 || config.Width <= config.Height && config.Height == 720)
}

func (s *ScenicSpotPhotoStore) cache(ctx context.Context, source string) (string, error) {
	if !allowedScenicPhotoURL(source) {
		return "", errors.New("仅支持下载高德景点图片")
	}
	dst := filepath.Join(s.root, scenicPhotoCachePath(source))
	url := scenicPhotoCachedURL(source)
	if validScenicPhotoFile(dst) {
		return url, nil
	}
	result := s.flight.DoChan(source, func() (any, error) {
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if validScenicPhotoFile(dst) {
			return url, nil
		}
		if err := s.download(ctx, source, dst); err != nil {
			return nil, err
		}
		return url, nil
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return "", value.Err
		}
		return value.Val.(string), nil
	}
}

func (s *ScenicSpotPhotoStore) download(ctx context.Context, source, dst string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	encoder, err := exec.LookPath("cwebp")
	if err != nil {
		return errors.New("景点图片压缩组件不可用，请安装 cwebp")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return errors.New("景点图片地址无效")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return errors.New("高德景点图片下载失败或超时")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > scenicPhotoMaxBytes {
		return errors.New("高德景点图片下载失败或超过8MiB")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, scenicPhotoMaxBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > scenicPhotoMaxBytes {
		return errors.New("高德景点图片内容无效或超过8MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") || config.Width <= 0 || config.Height <= 0 || config.Width > 12000 || config.Height > 12000 || int64(config.Width)*int64(config.Height) > 24000000 {
		return errors.New("高德景点图片格式或尺寸无效")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return errors.New("景点图片目录创建失败")
	}
	work, err := os.MkdirTemp(filepath.Dir(dst), ".photo-*")
	if err != nil {
		return errors.New("景点图片临时目录创建失败")
	}
	defer os.RemoveAll(work)
	input, output := filepath.Join(work, "source"), filepath.Join(work, "compressed.webp")
	// Normalize EXIF before selecting the landscape/portrait size; discard stale orientation.
	if orientation := scenicPhotoOrientation(raw); orientation > 1 {
		decoded, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return errors.New("景点图片方向校正失败")
		}
		oriented := orientScenicPhoto(decoded, orientation)
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, oriented); err != nil {
			return errors.New("景点图片方向校正失败")
		}
		raw = buffer.Bytes()
		config.Width, config.Height = oriented.Bounds().Dx(), oriented.Bounds().Dy()
	}
	if err := os.WriteFile(input, raw, 0o600); err != nil {
		return errors.New("景点图片临时文件写入失败")
	}
	width, height := 1024, 0
	if config.Width < config.Height {
		width, height = 0, 720
	}
	command := exec.CommandContext(ctx, encoder, "-quiet", "-q", "75", "-m", "6", "-sharp_yuv", "-af", "-metadata", "icc", "-resize", strconv.Itoa(width), strconv.Itoa(height), input, "-o", output)
	if err := command.Run(); err != nil {
		return errors.New("景点图片压缩失败或超时")
	}
	if !validScenicPhotoFile(output) {
		return errors.New("景点图片压缩结果无效")
	}
	if err := os.Chmod(output, 0o644); err != nil {
		return errors.New("景点图片保存失败")
	}
	if err := os.Rename(output, dst); err != nil {
		return errors.New("景点图片保存失败")
	}
	return nil
}

func (s *ScenicSpotPhotoStore) localize(ctx context.Context, photos []models.ScenicSpotPhoto) ([]models.ScenicSpotPhoto, int, int) {
	result := append([]models.ScenicSpotPhoto(nil), photos...)
	if s == nil {
		return result, 0, 0
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cached, failed := 0, 0
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				photo := &result[index]
				source := safeAMapPhotoURL(photo.URL)
				if source == "" {
					continue
				}
				url, err := s.cache(ctx, source)
				mu.Lock()
				if err != nil {
					log.Printf("scenic photo localization failed: photo_id=%q err=%v", photo.ID, err)
					failed++
				} else {
					photo.URL = url
					if photo.SourceURL == "" {
						photo.SourceURL = source
					}
					cached++
				}
				mu.Unlock()
			}
		}()
	}
	for index := range result {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return result, cached, failed
}

func (s *ScenicSpotCatalogService) localizePhotosForWrite(ctx context.Context, photos []models.ScenicSpotPhoto) ([]models.ScenicSpotPhoto, error) {
	localized, _, failed := s.photoStore.localize(ctx, photos)
	if failed > 0 {
		return nil, invalidScenicSpot("高德图片下载压缩失败，请重试；已保存图片未改变")
	}
	return localized, nil
}

// Read only the bounded EXIF IFD0 orientation, never arbitrary TIFF offsets/pixel data.
func scenicPhotoOrientation(raw []byte) int {
	var exif []byte
	switch {
	case len(raw) >= 2 && raw[0] == 0xff && raw[1] == 0xd8:
		for offset := 2; offset+4 <= len(raw) && raw[offset] == 0xff; {
			marker := raw[offset+1]
			if marker == 0xda || marker == 0xd9 {
				break
			}
			size := int(binary.BigEndian.Uint16(raw[offset+2:]))
			if size < 2 || offset+2+size > len(raw) {
				break
			}
			chunk := raw[offset+4 : offset+2+size]
			if marker == 0xe1 && bytes.HasPrefix(chunk, []byte("Exif\x00\x00")) {
				exif = chunk[6:]
				break
			}
			offset += 2 + size
		}
	case len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		for offset := 12; offset+8 <= len(raw); {
			size := int64(binary.LittleEndian.Uint32(raw[offset+4:]))
			if size > int64(len(raw)-offset-8) {
				break
			}
			if string(raw[offset:offset+4]) == "EXIF" {
				exif = raw[offset+8 : offset+8+int(size)]
				break
			}
			offset += 8 + int(size) + int(size%2)
		}
	case len(raw) >= 8 && bytes.Equal(raw[:8], []byte("\x89PNG\r\n\x1a\n")):
		for offset := 8; offset+12 <= len(raw); {
			size := int64(binary.BigEndian.Uint32(raw[offset:]))
			if size > int64(len(raw)-offset-12) {
				break
			}
			if string(raw[offset+4:offset+8]) == "eXIf" {
				exif = raw[offset+8 : offset+8+int(size)]
				break
			}
			offset += 12 + int(size)
		}
	}
	if bytes.HasPrefix(exif, []byte("Exif\x00\x00")) {
		exif = exif[6:]
	}
	if len(exif) < 8 {
		return 1
	}
	var order binary.ByteOrder
	if string(exif[:2]) == "II" {
		order = binary.LittleEndian
	} else if string(exif[:2]) == "MM" {
		order = binary.BigEndian
	} else {
		return 1
	}
	if order.Uint16(exif[2:]) != 42 {
		return 1
	}
	offset := int64(order.Uint32(exif[4:]))
	if offset < 8 || offset+2 > int64(len(exif)) {
		return 1
	}
	count := int64(order.Uint16(exif[offset:]))
	for index := int64(0); index < count && offset+2+(index+1)*12 <= int64(len(exif)); index++ {
		entry := exif[offset+2+index*12:]
		if order.Uint16(entry) == 0x0112 && order.Uint16(entry[2:]) == 3 && order.Uint32(entry[4:]) == 1 {
			value := int(order.Uint16(entry[8:]))
			if value >= 1 && value <= 8 {
				return value
			}
		}
	}
	return 1
}

func orientScenicPhoto(source image.Image, orientation int) *image.NRGBA {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	width, height := w, h
	if orientation >= 5 {
		width, height = h, w
	}
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			result.Set(dx, dy, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return result
}
