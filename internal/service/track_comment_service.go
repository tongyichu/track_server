package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

type CommentObjectStore interface {
	CommentUpload(context.Context, int64, string) (*CommentUpload, error)
	VerifyCommentImage(context.Context, *models.CommentImage) error
	DownloadCommentImage(int64, string, string) error
}
type CommentUpload struct {
	ImageID    string                  `json:"image_id"`
	ObjectKey  string                  `json:"object_key"`
	OSSURL     string                  `json:"-"`
	Credential *OSSTemporaryCredential `json:"credential"`
}
type CommentService struct {
	repo         repository.CommentRepository
	tracks       repository.TrackRepository
	users        *UserService
	objects      CommentObjectStore
	cache        *AssetCacheService
	restrictions *AccountRestrictionService
}

func NewCommentService(repo repository.CommentRepository, tracks repository.TrackRepository, users *UserService) *CommentService {
	return &CommentService{repo: repo, tracks: tracks, users: users}
}
func (s *CommentService) SetRestrictions(restrictions *AccountRestrictionService) {
	s.restrictions = restrictions
}
func (s *CommentService) allowed(ctx context.Context, user int64) error {
	if s.restrictions == nil {
		return nil
	}
	return s.restrictions.EnsureAllowed(ctx, user, "账号已被限制，禁止评论或评价评论")
}
func (s *CommentService) SetImages(objects CommentObjectStore, cache *AssetCacheService) {
	s.objects = objects
	s.cache = cache
	if cache != nil && objects != nil {
		cache.SetDownloader(commentImageDownloader{objects})
	}
}

type CommentInput struct {
	ClientID  string   `json:"client_comment_id"`
	Content   string   `json:"content"`
	ReplyToID string   `json:"reply_to_id"`
	ImageIDs  []string `json:"image_ids"`
}
type CommentUser struct {
	ID        int64  `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}
type CommentMedia struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type CommentQuote struct {
	ID        string      `json:"id"`
	User      CommentUser `json:"user"`
	Content   string      `json:"content"`
	Deleted   bool        `json:"deleted"`
	HasImages bool        `json:"has_images"`
}
type CommentView struct {
	CommentCount *int64         `json:"comment_count,omitempty"`
	ID           string         `json:"id"`
	TrackID      string         `json:"track_id"`
	RootID       string         `json:"root_id"`
	User         CommentUser    `json:"user"`
	Content      string         `json:"content"`
	Images       []CommentMedia `json:"images"`
	Deleted      bool           `json:"deleted"`
	IsAuthor     bool           `json:"is_author"`
	CanDelete    bool           `json:"can_delete"`
	ReplyCount   int64          `json:"reply_count"`
	LikeCount    int64          `json:"like_count"`
	DislikeCount int64          `json:"dislike_count"`
	Reaction     int            `json:"reaction"`
	CreatedAt    time.Time      `json:"created_at"`
	Quote        *CommentQuote  `json:"quote,omitempty"`
	AuthorReply  *CommentView   `json:"author_reply,omitempty"`
}
type CommentPage struct {
	Items      []*CommentView `json:"items"`
	Root       *CommentView   `json:"root,omitempty"`
	TotalCount int64          `json:"total_count"`
	HasMore    bool           `json:"has_more"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func newCommentID(prefix string) (string, error) {
	var data [16]byte
	if _, err := io.ReadFull(rand.Reader, data[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(data[:]), nil
}
func (s *CommentService) eligible(ctx context.Context, user int64, trackID string) (*models.Track, error) {
	if user <= 0 {
		return nil, repository.ErrForbidden
	}
	track, err := s.tracks.FindByID(ctx, trackID)
	if err != nil {
		return nil, err
	}
	if !likeEligible(track) {
		return nil, repository.ErrNotFound
	}
	return track, nil
}
func (s *CommentService) Create(ctx context.Context, user int64, trackID string, input CommentInput) (*CommentView, error) {
	if err := s.allowed(ctx, user); err != nil {
		return nil, err
	}
	track, err := s.eligible(ctx, user, trackID)
	if err != nil {
		return nil, err
	}
	if input.ImageIDs == nil {
		input.ImageIDs = []string{}
	}
	input.Content = strings.TrimSpace(input.Content)
	input.ClientID = strings.ToLower(strings.TrimSpace(input.ClientID))
	for _, ch := range input.ClientID {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return nil, invalidArg("invalid client_comment_id")
		}
	}
	if len(input.ClientID) < 8 || len(input.ClientID) > 64 || utf8.RuneCountInString(input.Content) > 1000 || len(input.ImageIDs) > 1 || (input.Content == "" && len(input.ImageIDs) == 0) {
		return nil, invalidArg("invalid comment content or client_comment_id")
	}
	seen := map[string]bool{}
	for _, id := range input.ImageIDs {
		if id == "" || seen[id] {
			return nil, invalidArg("invalid image_ids")
		}
		seen[id] = true
	}
	canonical, _ := json.Marshal(input)
	hash := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(hash[:])
	old, err := s.repo.FindClient(ctx, user, input.ClientID)
	if err == nil {
		if old.TrackID != trackID || old.RequestHash != fingerprint {
			return nil, repository.ErrCommentConflict
		}
		return s.one(ctx, user, track, old)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	for _, id := range input.ImageIDs {
		image, err := s.repo.Image(ctx, id)
		if err != nil {
			return nil, err
		}
		if image.UserID != user || image.TrackID != trackID || image.CommentID != "" {
			return nil, repository.ErrForbidden
		}
		if s.objects == nil {
			return nil, repository.ErrCommentUnavailable
		}
		if err = s.objects.VerifyCommentImage(ctx, image); err != nil {
			return nil, err
		}
	}
	id, err := newCommentID("TC.")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	c := &models.TrackComment{ID: id, TrackID: trackID, ReplyToID: input.ReplyToID, UserID: user, ClientID: input.ClientID, RequestHash: fingerprint, Content: input.Content, ImageIDs: input.ImageIDs, CreatedAt: now, UpdatedAt: now}
	result, err := s.repo.Apply(ctx, repository.CommentMutation{Action: "create", TrackID: trackID, UserID: user, Comment: c})
	if err != nil {
		if existing, lookupErr := s.repo.FindClient(ctx, user, input.ClientID); lookupErr == nil {
			if existing.TrackID != trackID || existing.RequestHash != fingerprint {
				return nil, repository.ErrCommentConflict
			}
			return s.one(ctx, user, track, existing)
		}
		return nil, err
	}
	return s.one(ctx, user, track, result)
}
func (s *CommentService) Delete(ctx context.Context, user int64, trackID, id string) (int64, error) {
	// Owners may still remove their own comment after a track becomes private or is deleted.
	if user <= 0 {
		return 0, repository.ErrForbidden
	}
	_, err := s.repo.Apply(ctx, repository.CommentMutation{Action: "delete", TrackID: trackID, UserID: user, CommentID: id})
	if err != nil {
		return 0, err
	}
	return s.repo.Total(ctx, trackID)
}
func (s *CommentService) React(ctx context.Context, user int64, trackID, id string, reaction int) (*CommentView, error) {
	if reaction != 0 {
		if err := s.allowed(ctx, user); err != nil {
			return nil, err
		}
	}
	track, err := s.eligible(ctx, user, trackID)
	if err != nil {
		return nil, err
	}
	if reaction < -1 || reaction > 1 {
		return nil, invalidArg("reaction must be -1, 0 or 1")
	}
	c, err := s.repo.Apply(ctx, repository.CommentMutation{Action: "reaction", TrackID: trackID, UserID: user, CommentID: id, Reaction: reaction})
	if err != nil {
		return nil, err
	}
	return s.one(ctx, user, track, c)
}
func (s *CommentService) Page(ctx context.Context, user int64, trackID, rootID, cursor string, limit int) (*CommentPage, error) {
	track, err := s.eligible(ctx, user, trackID)
	if err != nil {
		return nil, err
	}
	var root *models.TrackComment
	if rootID != "" {
		root, err = s.repo.Find(ctx, rootID)
		if err != nil {
			return nil, err
		}
		if root.TrackID != trackID || root.RootID != "" || (root.Deleted && root.ReplyCount == 0) {
			return nil, repository.ErrNotFound
		}
	}
	var c *models.CommentCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		c = &models.CommentCursor{}
		if err != nil || json.Unmarshal(raw, c) != nil || c.ID == "" || c.CreatedAt.IsZero() || c.TrackID != trackID || c.RootID != rootID {
			return nil, invalidArg("invalid comment cursor")
		}
	}
	if limit < 0 {
		return nil, invalidArg("invalid limit")
	}
	limit = normalizeTrackPageLimit(limit)
	rows, err := s.repo.List(ctx, trackID, rootID, c, limit+1)
	if err != nil {
		return nil, err
	}
	page := &CommentPage{Items: []*CommentView{}, HasMore: len(rows) > limit}
	if page.HasMore {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(models.CommentCursor{TrackID: trackID, RootID: rootID, CreatedAt: last.CreatedAt, ID: last.ID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	all := append([]*models.TrackComment{}, rows...)
	previews := map[string]*models.TrackComment{}
	if root == nil {
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		previews, err = s.repo.AuthorReplies(ctx, track.UserID, ids)
		if err != nil {
			return nil, err
		}
		for _, preview := range previews {
			all = append(all, preview)
		}
		page.TotalCount, err = s.repo.Total(ctx, trackID)
	} else {
		all = append(all, root)
		page.TotalCount = root.ReplyCount
	}
	if err != nil {
		return nil, err
	}
	views, err := s.decorate(ctx, user, track, all)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		view := views[row.ID]
		if p := previews[row.ID]; p != nil {
			view.AuthorReply = views[p.ID]
		}
		page.Items = append(page.Items, view)
	}
	if root != nil {
		page.Root = views[root.ID]
	}
	return page, nil
}
func (s *CommentService) Detail(ctx context.Context, user int64, trackID, id string) (*CommentView, error) {
	track, err := s.eligible(ctx, user, trackID)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.TrackID != trackID || (c.Deleted && (c.RootID != "" || c.ReplyCount == 0)) {
		return nil, repository.ErrNotFound
	}
	return s.one(ctx, user, track, c)
}
func (s *CommentService) one(ctx context.Context, user int64, track *models.Track, c *models.TrackComment) (*CommentView, error) {
	rows := []*models.TrackComment{c}
	var preview *models.TrackComment
	if c.RootID == "" {
		previews, err := s.repo.AuthorReplies(ctx, track.UserID, []string{c.ID})
		if err != nil {
			return nil, err
		}
		preview = previews[c.ID]
		if preview != nil {
			rows = append(rows, preview)
		}
	}
	views, err := s.decorate(ctx, user, track, rows)
	if err != nil {
		return nil, err
	}
	if preview != nil {
		views[c.ID].AuthorReply = views[preview.ID]
	}
	count, err := s.repo.Total(ctx, track.ID)
	if err != nil {
		return nil, err
	}
	views[c.ID].CommentCount = &count
	return views[c.ID], nil
}
func (s *CommentService) decorate(ctx context.Context, viewer int64, track *models.Track, rows []*models.TrackComment) (map[string]*CommentView, error) {
	quoteIDs := []string{}
	userIDs := []int64{}
	ids := []string{}
	for _, c := range rows {
		ids = append(ids, c.ID)
		userIDs = append(userIDs, c.UserID)
		if c.ReplyToID != "" && c.ReplyToID != c.RootID {
			quoteIDs = append(quoteIDs, c.ReplyToID)
		}
	}
	quotes, err := s.repo.FindMany(ctx, quoteIDs)
	if err != nil {
		return nil, err
	}
	for _, q := range quotes {
		userIDs = append(userIDs, q.UserID)
	}
	byUser, err := s.commentUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	reactions, err := s.repo.Reactions(ctx, viewer, ids)
	if err != nil {
		return nil, err
	}
	out := map[string]*CommentView{}
	for _, c := range rows {
		u := byUser[c.UserID]
		if u.ID == 0 {
			u.ID = c.UserID
		}
		view := &CommentView{ID: c.ID, TrackID: c.TrackID, RootID: c.RootID, User: u, Content: c.Content, Images: []CommentMedia{}, Deleted: c.Deleted, IsAuthor: c.UserID == track.UserID, CanDelete: c.UserID == viewer && !c.Deleted, ReplyCount: c.ReplyCount, LikeCount: c.LikeCount, DislikeCount: c.DislikeCount, Reaction: reactions[c.ID], CreatedAt: c.CreatedAt}
		if c.Deleted {
			view.Content = ""
			view.LikeCount = 0
			view.DislikeCount = 0
			view.Reaction = 0
		} else {
			for _, media := range c.Images {
				view.Images = append(view.Images, CommentMedia{media.ID, fmt.Sprintf("/api/v1/track/%s/comments/%s/images/%s", c.TrackID, c.ID, media.ID), media.Width, media.Height})
			}
		}
		if q := quotes[c.ReplyToID]; q != nil {
			view.Quote = &CommentQuote{ID: q.ID, User: byUser[q.UserID], Content: q.Content, Deleted: q.Deleted, HasImages: len(q.ImageIDs) > 0}
			if q.Deleted {
				view.Quote.Content = ""
				view.Quote.HasImages = false
			}
		}
		out[c.ID] = view
	}
	return out, nil
}
func (s *CommentService) PrepareImage(ctx context.Context, user int64, trackID string, width, height int, size int64) (*CommentUpload, error) {
	if err := s.allowed(ctx, user); err != nil {
		return nil, err
	}
	if _, err := s.eligible(ctx, user, trackID); err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 || size <= 0 || size > 5*1024*1024 {
		return nil, invalidArg("invalid image dimensions or size")
	}
	if s.objects == nil {
		return nil, repository.ErrCommentUnavailable
	}
	id, err := newCommentID("CI.")
	if err != nil {
		return nil, err
	}
	upload, err := s.objects.CommentUpload(ctx, user, id)
	if err != nil {
		return nil, err
	}
	image := &models.CommentImage{ID: id, TrackID: trackID, UserID: user, ObjectKey: upload.ObjectKey, OSSURL: upload.OSSURL, Width: width, Height: height, ByteSize: size, CreatedAt: time.Now().UTC()}
	if err = s.repo.PutImage(ctx, image); err != nil {
		return nil, err
	}
	upload.ImageID = id
	return upload, nil
}
func (s *CommentService) ImageFile(ctx context.Context, user int64, trackID, commentID, imageID string) (string, error) {
	if _, err := s.eligible(ctx, user, trackID); err != nil {
		return "", err
	}
	c, err := s.repo.Find(ctx, commentID)
	if err != nil {
		return "", err
	}
	if c.TrackID != trackID || c.Deleted {
		return "", repository.ErrNotFound
	}
	image, err := s.repo.Image(ctx, imageID)
	if err != nil {
		return "", err
	}
	if image.TrackID != trackID || image.CommentID != commentID {
		return "", repository.ErrNotFound
	}
	if s.cache == nil {
		return "", repository.ErrCommentUnavailable
	}
	return s.cache.EnsureCachedFile(ctx, user, image.ID, image.OSSURL)
}
