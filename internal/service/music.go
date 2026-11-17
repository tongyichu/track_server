package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ErrMusicInput = errors.New("音乐资料无效")
var musicHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

type MusicService struct {
	repo      repository.MusicRepository
	directory string
}

func NewMusicService(repo repository.MusicRepository, directory string) *MusicService {
	return &MusicService{repo, directory}
}
func (s *MusicService) List(ctx context.Context, all bool) ([]models.MusicTrack, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("music unavailable")
	}
	items, err := s.repo.ListMusic(ctx)
	if err != nil {
		return nil, err
	}
	out := []models.MusicTrack{}
	for _, v := range items {
		if all || v.Published {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *MusicService) Find(ctx context.Context, id string, all bool) (models.MusicTrack, error) {
	items, err := s.List(ctx, all)
	if err != nil {
		return models.MusicTrack{}, err
	}
	for _, v := range items {
		if v.ID == id {
			return v, nil
		}
	}
	return models.MusicTrack{}, repository.ErrNotFound
}
func (s *MusicService) Asset(ctx context.Context, id, kind, hash string, all bool) (string, string, error) {
	v, err := s.Find(ctx, id, all)
	if err != nil {
		return "", "", err
	}
	if kind == "audio" && v.AudioLink != "" {
		return "", "", repository.ErrNotFound
	}
	want, ext, mime := v.AudioSHA, ".mp3", "audio/mpeg"
	if kind == "cover" {
		want, ext, mime = v.CoverSHA, ".jpg", "image/jpeg"
	} else if kind != "audio" {
		return "", "", repository.ErrNotFound
	}
	if !musicHash.MatchString(want) || hash != want {
		return "", "", repository.ErrNotFound
	}
	return filepath.Join(s.directory, want+ext), mime, nil
}
func (s *MusicService) Save(ctx context.Context, v models.MusicTrack, audio, cover []byte) (models.MusicTrack, error) {
	items, err := s.List(ctx, true)
	if err != nil {
		return v, err
	}
	previous := v.Revision
	requestedDuration := v.DurationMS
	requestedLink := strings.TrimSpace(v.AudioLink)
	if len(audio) > 0 && requestedLink != "" {
		return v, fmt.Errorf("%w: 文件与链接请选择一种音频来源", ErrMusicInput)
	}
	if previous < 0 || v.SortOrder < -9999 || v.SortOrder > 9999 {
		return v, ErrMusicInput
	}
	v.Name = strings.TrimSpace(v.Name)
	v.Artist = strings.TrimSpace(v.Artist)
	if v.Name == "" || len(v.Name) > 240 || v.Artist == "" || len(v.Artist) > 240 || len(v.Source) > 500 || len(v.Attribution) > 1200 || len(v.License) > 100 {
		return v, fmt.Errorf("%w: 请填写名称与歌手，且资料长度不能超限", ErrMusicInput)
	}
	if v.ID == "" {
		v.AudioSHA, v.CoverSHA = "", ""
		v.AudioVersion = ""
		v.AudioLink = ""
		v.AudioBytes, v.DurationMS = 0, 0
		if len(items) >= 500 {
			return v, fmt.Errorf("%w: 乐库最多500首，请先整理现有条目", ErrMusicInput)
		}
		if previous != 0 {
			return v, repository.ErrMusicConflict
		}
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return v, err
		}
		v.ID = hex.EncodeToString(b)
	} else {
		old, e := s.Find(ctx, v.ID, true)
		if e != nil {
			return v, e
		}
		if old.Revision != previous {
			return v, repository.ErrMusicConflict
		}
		v.AudioSHA, v.AudioBytes, v.DurationMS, v.CoverSHA = old.AudioSHA, old.AudioBytes, old.DurationMS, old.CoverSHA
		v.AudioVersion = old.AudioVersion
		if requestedLink != old.AudioLink {
			v.DurationMS = 0
		}
		if requestedLink != "" && requestedLink == old.AudioLink && !v.Published {
			v.AudioLink = old.AudioLink
		} else {
			v.AudioLink = ""
		}
	}
	if requestedLink != "" {
		if requestedDuration != 0 && (requestedDuration < 1000 || requestedDuration > 1200000) {
			return v, fmt.Errorf("%w: 时长应在20分钟以内", ErrMusicInput)
		}
		if v.AudioLink == "" || v.Published {
			info, e := probeMusicLink(ctx, requestedLink)
			if e != nil {
				return v, e
			}
			v.AudioLink, v.AudioBytes, v.AudioVersion = info.URL, info.Bytes, info.Version
		}
		v.AudioSHA = ""
		if requestedDuration > 0 || v.DurationMS == 0 {
			v.DurationMS = requestedDuration
		}
	} else if len(audio) == 0 && v.AudioSHA == "" {
		return v, fmt.Errorf("%w: 请上传MP3音频或填写音频链接", ErrMusicInput)
	} else {
		v.AudioLink = ""
		v.AudioVersion = ""
	}
	if len(audio) > 0 {
		duration, e := musicMP3Duration(audio)
		if e != nil {
			return v, fmt.Errorf("%w: 仅支持有效MP3，时长1秒至20分钟，最大30MB", ErrMusicInput)
		}
		v.DurationMS = duration
		v.AudioBytes = int64(len(audio))
		v.AudioSHA, err = s.write(audio, ".mp3")
		if err != nil {
			return v, err
		}
	}
	if v.AudioSHA == "" && v.AudioLink == "" {
		return v, fmt.Errorf("%w: 请上传MP3音频", ErrMusicInput)
	}
	if v.StartOffsetMS < 0 || v.StartOffsetMS >= 1200000 {
		return v, fmt.Errorf("%w: 起播时间应在0秒至20分钟之间", ErrMusicInput)
	}
	if v.StartOffsetMS > 0 && v.DurationMS == 0 {
		return v, fmt.Errorf("%w: 请先试听读取音乐时长，再设置起播时间", ErrMusicInput)
	}
	if v.DurationMS > 0 && v.StartOffsetMS >= v.DurationMS {
		return v, fmt.Errorf("%w: 起播时间必须小于音乐时长", ErrMusicInput)
	}
	if len(cover) > 0 {
		cfg, _, e := image.DecodeConfig(bytes.NewReader(cover))
		if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
			return v, fmt.Errorf("%w: 封面须为2048像素以内的PNG/JPEG", ErrMusicInput)
		}
		img, _, e := image.Decode(bytes.NewReader(cover))
		if e != nil {
			return v, ErrMusicInput
		}
		var out bytes.Buffer
		if e = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); e != nil {
			return v, e
		}
		v.CoverSHA, err = s.write(out.Bytes(), ".jpg")
		if err != nil {
			return v, err
		}
	}
	v.Revision = previous + 1
	v.UpdatedAt = time.Now().UTC()
	err = s.repo.SaveMusic(ctx, v, previous)
	return v, err
}
func (s *MusicService) write(data []byte, ext string) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if err := os.MkdirAll(s.directory, 0750); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.directory, ".upload-")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return hash, os.Rename(f.Name(), filepath.Join(s.directory, hash+ext))
}

// Count MPEG audio frames instead of trusting file extensions or a supplied duration.
func musicMP3Duration(data []byte) (int64, error) {
	if len(data) > 30*1024*1024 || len(data) < 4 {
		return 0, ErrMusicInput
	}
	pos := 0
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		for _, b := range data[6:10] {
			if b&128 != 0 {
				return 0, ErrMusicInput
			}
		}
		pos = 10 + (int(data[6]) << 21) + (int(data[7]) << 14) + (int(data[8]) << 7) + int(data[9])
		if pos > len(data) {
			return 0, ErrMusicInput
		}
		if data[5]&16 != 0 {
			pos += 10
		}
	}
	var seconds float64
	frames := 0
	for pos+4 <= len(data) {
		if len(data)-pos == 128 && string(data[pos:pos+3]) == "TAG" {
			break
		}
		b := data[pos:]
		if b[0] != 255 || b[1]&224 != 224 {
			return 0, ErrMusicInput
		}
		version := int(b[1] >> 3 & 3)
		layer := b[1] >> 1 & 3
		rate := int(b[2] >> 2 & 3)
		bit := int(b[2] >> 4)
		if version == 1 || layer != 1 || rate == 3 || bit == 0 || bit == 15 {
			return 0, ErrMusicInput
		}
		frequency := []int{44100, 48000, 32000}[rate]
		samples := 1152
		kbps := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}[bit]
		coefficient := 144000
		if version != 3 {
			frequency /= 2
			if version == 0 {
				frequency /= 2
			}
			samples = 576
			kbps = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}[bit]
			coefficient = 72000
		}
		size := coefficient*kbps/frequency + int(b[2]>>1&1)
		if size < 4 || pos+size > len(data) {
			return 0, ErrMusicInput
		}
		pos += size
		seconds += float64(samples) / float64(frequency)
		frames++
	}
	duration := int64(seconds * 1000)
	if frames < 2 || duration < 1000 || duration > 1200000 {
		return 0, ErrMusicInput
	}
	return duration, nil
}
