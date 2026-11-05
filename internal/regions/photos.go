package regions

import (
	"errors"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxPhotos = 20

func validatePhotos(photos []Photo) error {
	if len(photos) > MaxPhotos {
		return errors.New("每个地区最多保存20张图片")
	}
	seen := make(map[string]bool, len(photos))
	for _, photo := range photos {
		if photo.ID == "" || len(photo.ID) > 128 || seen[photo.ID] {
			return errors.New("图片ID无效或重复")
		}
		seen[photo.ID] = true
		if len(photo.URL) > 2048 || strings.ContainsAny(photo.URL, "\x00\r\n\\") {
			return errors.New("图片地址无效")
		}
		u, err := url.Parse(photo.URL)
		if err != nil || u.User != nil {
			return errors.New("图片地址无效")
		}
		local := u.Scheme == "" && u.Host == "" && u.RawQuery == "" && u.Fragment == "" &&
			strings.HasPrefix(u.Path, "/api/v1/static/regions/") && path.Clean(u.Path) == u.Path
		if !local && !(u.Scheme == "https" && u.Host != "") {
			return errors.New("图片须使用地区上传地址或HTTPS地址")
		}
		if utf8.RuneCountInString(photo.Caption) > 500 || utf8.RuneCountInString(photo.AltText) > 500 {
			return errors.New("图片说明不能超过500字")
		}
	}
	return nil
}
