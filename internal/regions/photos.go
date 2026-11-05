package regions

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxPhotos = 20

// SavePhotos changes only the photo field and checks its own baseline. Text
// changes in the latest catalog are preserved, including across CAS retries.
func (s *Service) SavePhotos(ctx context.Context, id string, base, photos []Photo) (*Node, int64, error) {
	if err := validatePhotos(photos); err != nil {
		return nil, 0, err
	}
	equal := func(a, b []Photo) bool {
		left, _ := json.Marshal(append([]Photo{}, a...))
		right, _ := json.Marshal(append([]Photo{}, b...))
		return string(left) == string(right)
	}
	for attempt := 0; attempt < 4; attempt++ {
		catalog, err := s.Read(ctx)
		if err != nil {
			return nil, 0, err
		}
		index := -1
		for i := range catalog.Nodes {
			if catalog.Nodes[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, 0, ErrNodeConflict
		}
		node := &catalog.Nodes[index]
		if node.Level != "province" && node.Level != "city" {
			return nil, 0, errors.New("仅省份和城市支持图片")
		}
		if equal(node.Photos, photos) {
			return node, catalog.Version, nil
		}
		if !equal(node.Photos, base) {
			return nil, 0, ErrNodeConflict
		}
		node.Photos = append([]Photo(nil), photos...)
		saved, err := s.Save(ctx, catalog.Version, catalog.Nodes)
		if errors.Is(err, ErrVersionConflict) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		return &saved.Nodes[index], saved.Version, nil
	}
	return nil, 0, ErrVersionConflict
}

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
