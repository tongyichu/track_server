package repository

import (
	"context"
	"github.com/tongyichu/track_server/internal/models"
	"sort"
	"sync"
)

type MemoryCommentRepository struct {
	mu        sync.RWMutex
	comments  map[string]*models.TrackComment
	clients   map[commentClient]string
	images    map[string]*models.CommentImage
	totals    map[string]int64
	reactions map[commentClient]int
}
type commentClient struct {
	User int64
	Key  string
}

func NewMemoryCommentRepository() *MemoryCommentRepository {
	return &MemoryCommentRepository{comments: map[string]*models.TrackComment{}, clients: map[commentClient]string{}, images: map[string]*models.CommentImage{}, totals: map[string]int64{}, reactions: map[commentClient]int{}}
}
func copyComment(c *models.TrackComment) *models.TrackComment {
	copy := *c
	copy.ImageIDs = append([]string{}, c.ImageIDs...)
	copy.Images = append([]models.CommentMedia{}, c.Images...)
	return &copy
}
func (r *MemoryCommentRepository) Apply(_ context.Context, m CommentMutation) (*models.TrackComment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return applyCommentMutation(memoryCommentTx{r}, m)
}

type memoryCommentTx struct{ r *MemoryCommentRepository }

func (t memoryCommentTx) Find(id string) (*models.TrackComment, error) {
	c := t.r.comments[id]
	if c == nil {
		return nil, ErrNotFound
	}
	return copyComment(c), nil
}
func (t memoryCommentTx) FindClient(user int64, key string) (*models.TrackComment, error) {
	return t.Find(t.r.clients[commentClient{user, key}])
}
func (t memoryCommentTx) Save(c *models.TrackComment) error {
	t.r.comments[c.ID] = copyComment(c)
	t.r.clients[commentClient{c.UserID, c.ClientID}] = c.ID
	return nil
}
func (t memoryCommentTx) Image(id string) (*models.CommentImage, error) {
	image := t.r.images[id]
	if image == nil {
		return nil, ErrNotFound
	}
	copy := *image
	return &copy, nil
}
func (t memoryCommentTx) SaveImage(image *models.CommentImage) error {
	copy := *image
	t.r.images[image.ID] = &copy
	return nil
}
func (t memoryCommentTx) AddTotal(track string, delta int64) error {
	t.r.totals[track] += delta
	return nil
}
func (t memoryCommentTx) Reaction(id string, user int64) (int, error) {
	return t.r.reactions[commentClient{user, id}], nil
}
func (t memoryCommentTx) SetReaction(id string, user int64, value int) error {
	t.r.reactions[commentClient{user, id}] = value
	return nil
}
func (r *MemoryCommentRepository) Find(_ context.Context, id string) (*models.TrackComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return (memoryCommentTx{r}).Find(id)
}
func (r *MemoryCommentRepository) FindClient(_ context.Context, user int64, key string) (*models.TrackComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return (memoryCommentTx{r}).FindClient(user, key)
}
func (r *MemoryCommentRepository) FindMany(_ context.Context, ids []string) (map[string]*models.TrackComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]*models.TrackComment{}
	for _, id := range ids {
		if c := r.comments[id]; c != nil {
			out[id] = copyComment(c)
		}
	}
	return out, nil
}
func commentNewer(a, b *models.TrackComment) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}
func (r *MemoryCommentRepository) List(_ context.Context, track, root string, cursor *models.CommentCursor, limit int) ([]*models.TrackComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []*models.TrackComment{}
	for _, c := range r.comments {
		if c.TrackID != track || c.RootID != root || (c.Deleted && (root != "" || c.ReplyCount == 0)) {
			continue
		}
		if cursor != nil && !commentNewer(&models.TrackComment{CreatedAt: cursor.CreatedAt, ID: cursor.ID}, c) {
			continue
		}
		out = append(out, copyComment(c))
	}
	sort.Slice(out, func(i, j int) bool { return commentNewer(out[i], out[j]) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *MemoryCommentRepository) Total(_ context.Context, track string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.totals[track], nil
}
func (r *MemoryCommentRepository) Totals(_ context.Context, tracks []string) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	counts := make(map[string]int64, len(tracks))
	for _, id := range tracks {
		counts[id] = r.totals[id]
	}
	return counts, nil
}
func (r *MemoryCommentRepository) AuthorReplies(_ context.Context, user int64, roots []string) (map[string]*models.TrackComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	wanted := map[string]bool{}
	for _, id := range roots {
		wanted[id] = true
	}
	out := map[string]*models.TrackComment{}
	for _, c := range r.comments {
		if c.Deleted || c.UserID != user || !wanted[c.RootID] {
			continue
		}
		if out[c.RootID] == nil || commentNewer(c, out[c.RootID]) {
			out[c.RootID] = copyComment(c)
		}
	}
	return out, nil
}
func (r *MemoryCommentRepository) Reactions(_ context.Context, user int64, ids []string) (map[string]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]int{}
	for _, id := range ids {
		out[id] = r.reactions[commentClient{user, id}]
	}
	return out, nil
}
func (r *MemoryCommentRepository) Likers(_ context.Context, id string, before int64, limit int) ([]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	users := []int64{}
	for key, reaction := range r.reactions {
		if key.Key == id && reaction == 1 && (before == 0 || key.User < before) {
			users = append(users, key.User)
		}
	}
	sort.Slice(users, func(i, j int) bool { return users[i] > users[j] })
	if len(users) > limit {
		users = users[:limit]
	}
	return users, nil
}
func (r *MemoryCommentRepository) PutImage(_ context.Context, image *models.CommentImage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return (memoryCommentTx{r}).SaveImage(image)
}
func (r *MemoryCommentRepository) Image(_ context.Context, id string) (*models.CommentImage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return (memoryCommentTx{r}).Image(id)
}
