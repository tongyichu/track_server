package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OSSObjectDownloader 抽象了 OSS 对象下载能力。
// 采用接口形式便于测试，以及避免 AssetCacheService 强耦合到 OSSTokenService。
type OSSObjectDownloader interface {
	// DownloadObject 下载 ossObjectURL 指向的对象到 localPath。
	// 实现方需要自行处理鉴权（例如服务端 STS 临时凭证）。
	DownloadObject(userID int64, ossObjectURL, localPath string) error
}

// AssetCacheService 负责把 OSS 上的某类资源（截图 / 原始轨迹文件等）按需同步到
// 服务端本地缓存目录，并提供一个服务端可直接下载的 URL（由静态文件服务挂载）。
//
// 每一类资源对应一个独立实例：
//   - 截图：cacheDir=<LogDir>/screenshots, urlPrefix=/static/screenshots, 默认后缀 .png
//   - 原始轨迹：cacheDir=<LogDir>/raw_tracks, urlPrefix=/static/raw_tracks, 默认后缀 .dat
type AssetCacheService struct {
	// cacheDir 是服务端保存该类资源的绝对目录，所有文件都落在该目录下。
	cacheDir string
	// urlPrefix 是对外可下载的 URL 前缀，与静态文件服务挂载点保持一致。
	urlPrefix string
	// allowedExts 用于探测已缓存文件命中的后缀列表（按顺序尝试）。
	// 同时也用于从 sourceURL 解析扩展名时的白名单。
	allowedExts []string
	// defaultExt 当 sourceURL 里取不到允许列表内的后缀时作为兜底。
	defaultExt string
	// downloader 用于实际从 OSS 下载对象到本地磁盘。
	downloader OSSObjectDownloader

	// 同 key 并发下载合并。
	mu       sync.Mutex
	inflight map[string]*downloadTask
}

type downloadTask struct {
	done chan struct{}
	err  error
}

// NewAssetCacheService 构造资源缓存服务。
// - cacheDir：本地缓存目录，会在首次使用时自动创建。
// - urlPrefix：对外下载 URL 前缀（与路由静态挂载点一致）。
// - allowedExts：允许命中/识别的扩展名列表（小写，带 '.'）。空时用 []string{""} 表示无扩展名。
// - defaultExt：兜底扩展名，用于无法从 sourceURL 识别类型时。
func NewAssetCacheService(cacheDir, urlPrefix string, allowedExts []string, defaultExt string) (*AssetCacheService, error) {
	if cacheDir == "" {
		return nil, errors.New("cacheDir is required")
	}
	if urlPrefix == "" {
		return nil, errors.New("urlPrefix is required")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	if len(allowedExts) == 0 {
		allowedExts = []string{""}
	}
	return &AssetCacheService{
		cacheDir:    cacheDir,
		urlPrefix:   strings.TrimRight(urlPrefix, "/"),
		allowedExts: allowedExts,
		defaultExt:  defaultExt,
		inflight:    make(map[string]*downloadTask),
	}, nil
}

// SetDownloader 注入 OSS 下载实现。未注入时，EnsureCached 将只尝试命中本地缓存，
// 不会发起下载。
func (s *AssetCacheService) SetDownloader(d OSSObjectDownloader) {
	s.downloader = d
}

// GuessLocalURL 根据 sourceURL 的文件后缀规则，推导并返回服务端本地可下载 URL。
//
// 注意：该方法不会触发实际下载，也不会判断文件是否存在；仅用于在异步预热场景下
// 先把“服务端链接”下发给客户端（后续由 PrefetchAsync/EnsureCached 负责落盘）。
func (s *AssetCacheService) GuessLocalURL(key, sourceURL string) string {
	if key == "" || sourceURL == "" {
		return ""
	}
	ext := pickExt(sourceURL, s.allowedExts, s.defaultExt)
	return s.LocalURL(key, ext)
}

// LocalURL 返回给定 key 的本地下载 URL，文件是否存在不做保证。
func (s *AssetCacheService) LocalURL(key, ext string) string {
	return fmt.Sprintf("%s/%s", s.urlPrefix, fileName(key, ext))
}

// localPath 返回给定 key 对应的本地文件绝对路径。
func (s *AssetCacheService) localPath(key, ext string) string {
	return filepath.Join(s.cacheDir, fileName(key, ext))
}

// Exists 判断缓存文件是否已存在，返回命中的扩展名。
// 因为客户端上传的资源后缀不固定，这里按 allowedExts 顺序尝试匹配。
func (s *AssetCacheService) Exists(key string) (string, bool) {
	for _, ext := range s.allowedExts {
		p := s.localPath(key, ext)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return ext, true
		}
	}
	return "", false
}

// EnsureCached 确保 key 对应资源已缓存到本地，返回服务端可下载 URL。
// 若 sourceURL 为空或下载失败，返回空串（调用方据此决定是否兜底返回空）。
// userID 用于通过 STS 申请下载凭证。
func (s *AssetCacheService) EnsureCached(ctx context.Context, userID int64, key, sourceURL string) string {
	if key == "" {
		return ""
	}
	if ext, ok := s.Exists(key); ok {
		return s.LocalURL(key, ext)
	}
	if sourceURL == "" || s.downloader == nil {
		return ""
	}

	ext := pickExt(sourceURL, s.allowedExts, s.defaultExt)
	if err := s.downloadOnce(ctx, userID, key, ext, sourceURL); err != nil {
		return ""
	}
	return s.LocalURL(key, ext)
}

// PrefetchAsync 在后台预热资源，失败静默。
func (s *AssetCacheService) PrefetchAsync(userID int64, key, sourceURL string) {
	if key == "" || sourceURL == "" || s.downloader == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.EnsureCached(ctx, userID, key, sourceURL)
	}()
}

// downloadOnce 合并同 key 的并发下载，确保只下载一次。
func (s *AssetCacheService) downloadOnce(ctx context.Context, userID int64, key, ext, sourceURL string) error {
	s.mu.Lock()
	if task, ok := s.inflight[key]; ok {
		s.mu.Unlock()
		select {
		case <-task.done:
			return task.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	task := &downloadTask{done: make(chan struct{})}
	s.inflight[key] = task
	s.mu.Unlock()

	task.err = s.doDownload(userID, key, ext, sourceURL)
	close(task.done)

	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()
	return task.err
}

func (s *AssetCacheService) doDownload(userID int64, key, ext, sourceURL string) error {
	finalPath := s.localPath(key, ext)
	tmpPath := finalPath + ".tmp"
	// 先下载到 tmp 文件，成功后再原子 rename，避免半写文件被静态服务读到。
	if err := s.downloader.DownloadObject(userID, sourceURL, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if fi, err := os.Stat(tmpPath); err != nil || fi.Size() == 0 {
		_ = os.Remove(tmpPath)
		if err != nil {
			return err
		}
		return errors.New("downloaded asset is empty")
	}
	return os.Rename(tmpPath, finalPath)
}

// fileName 生成缓存文件名，保持 key 可作为主键。
func fileName(key, ext string) string {
	if ext == "" {
		return key
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return key + ext
}

// pickExt 从 sourceURL 中提取属于 allowed 白名单的扩展名；否则返回 defaultExt。
// 数据库中存放的 OSS URL 通常带大量签名查询参数，这里只取 Path 部分的扩展名。
func pickExt(sourceURL string, allowed []string, defaultExt string) string {
	u, err := url.Parse(sourceURL)
	if err == nil && u.Path != "" {
		ext := strings.ToLower(path.Ext(u.Path))
		for _, a := range allowed {
			if a != "" && a == ext {
				return ext
			}
		}
	}
	return defaultExt
}
