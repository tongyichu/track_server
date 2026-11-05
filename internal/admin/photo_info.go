package admin

import (
	"bytes"
	"image"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

// Photo file facts are read-only display data, separate from saved photo edits.
type photoImageInfo struct {
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	SizeBytes int64 `json:"size_bytes"`
}

func photoInfoFromBytes(raw []byte) photoImageInfo {
	info := photoImageInfo{SizeBytes: int64(len(raw))}
	if config, _, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil {
		info.Width, info.Height = config.Width, config.Height
	}
	return info
}

// Called only after the existing static asset session, module and path checks.
// It reads local image headers and never requests third-party photo URLs.
func servePhotoImageInfo(c *app.RequestContext, fullPath, rel string) {
	category := strings.Split(rel, "/")[0]
	ext := strings.ToLower(path.Ext(rel))
	if (category != "regions" && category != "scenic_spots") || (ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp") {
		c.JSON(http.StatusBadRequest, utils.H{"error": "仅支持查询省市或景点图片信息"})
		return
	}
	file, err := os.Open(fullPath)
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, utils.H{"error": "图片信息暂不可用"})
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片文件无效"})
		return
	}
	info := photoImageInfo{SizeBytes: stat.Size()}
	if config, _, err := image.DecodeConfig(io.LimitReader(file, maxScenicSpotPhotoSize)); err == nil {
		info.Width, info.Height = config.Width, config.Height
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": info})
}
