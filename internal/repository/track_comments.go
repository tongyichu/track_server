package repository

import (
	"context"
	"errors"
	"github.com/tongyichu/track_server/internal/models"
	"time"
)

var ErrCommentConflict = errors.New("comment request conflicts with an earlier submission")
var ErrCommentUnavailable = errors.New("comment storage is unavailable")

type CommentMutation struct {
	Action    string
	TrackID   string
	UserID    int64
	Comment   *models.TrackComment
	CommentID string
	Reaction  int
}
type CommentRepository interface {
	Apply(context.Context, CommentMutation) (*models.TrackComment, error)
	Find(context.Context, string) (*models.TrackComment, error)
	FindClient(context.Context, int64, string) (*models.TrackComment, error)
	FindMany(context.Context, []string) (map[string]*models.TrackComment, error)
	List(context.Context, string, string, *models.CommentCursor, int) ([]*models.TrackComment, error)
	Total(context.Context, string) (int64, error)
	Totals(context.Context, []string) (map[string]int64, error)
	AuthorReplies(context.Context, int64, []string) (map[string]*models.TrackComment, error)
	Reactions(context.Context, int64, []string) (map[string]int, error)
	Likers(context.Context, string, int64, int) ([]int64, error)
	PutImage(context.Context, *models.CommentImage) error
	Image(context.Context, string) (*models.CommentImage, error)
}
type commentTransaction interface {
	Find(string) (*models.TrackComment, error)
	FindClient(int64, string) (*models.TrackComment, error)
	Save(*models.TrackComment) error
	Image(string) (*models.CommentImage, error)
	SaveImage(*models.CommentImage) error
	AddTotal(string, int64) error
	Reaction(string, int64) (int, error)
	SetReaction(string, int64, int) error
}

// A mutation and all of its counters commit together. Adapters serialize writes for one track.
func applyCommentMutation(tx commentTransaction, m CommentMutation) (*models.TrackComment, error) {
	if m.Action == "create" {
		c := m.Comment
		old, err := tx.FindClient(c.UserID, c.ClientID)
		if err == nil {
			if old.TrackID != c.TrackID || old.RequestHash != c.RequestHash {
				return nil, ErrCommentConflict
			}
			return old, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		var root *models.TrackComment
		if c.ReplyToID != "" {
			target, err := tx.Find(c.ReplyToID)
			if err != nil {
				return nil, err
			}
			if target.TrackID != c.TrackID || target.Deleted {
				return nil, ErrNotFound
			}
			root = target
			if target.RootID != "" {
				root, err = tx.Find(target.RootID)
				if err != nil {
					return nil, err
				}
			}
			c.RootID = root.ID
		}
		images := make([]*models.CommentImage, 0, len(c.ImageIDs))
		for _, id := range c.ImageIDs {
			image, err := tx.Image(id)
			if err != nil {
				return nil, err
			}
			if image.UserID != c.UserID || image.TrackID != c.TrackID || image.CommentID != "" || c.CreatedAt.Sub(image.CreatedAt) > 24*time.Hour {
				return nil, ErrForbidden
			}
			images = append(images, image)
		}
		c.Images = []models.CommentMedia{}
		for _, image := range images {
			c.Images = append(c.Images, models.CommentMedia{ID: image.ID, Width: image.Width, Height: image.Height})
		}
		if err := tx.Save(c); err != nil {
			return nil, err
		}
		for _, image := range images {
			image.CommentID = c.ID
			if err := tx.SaveImage(image); err != nil {
				return nil, err
			}
		}
		if root != nil {
			root.ReplyCount++
			if err := tx.Save(root); err != nil {
				return nil, err
			}
		}
		if err := tx.AddTotal(c.TrackID, 1); err != nil {
			return nil, err
		}
		return c, nil
	}
	c, err := tx.Find(m.CommentID)
	if err != nil {
		return nil, err
	}
	if c.TrackID != m.TrackID {
		return nil, ErrNotFound
	}
	switch m.Action {
	case "delete":
		if c.UserID != m.UserID {
			return nil, ErrForbidden
		}
		if c.Deleted {
			return c, nil
		}
		c.Deleted = true
		c.Content = ""
		c.ImageIDs = []string{}
		c.Images = []models.CommentMedia{}
		c.UpdatedAt = time.Now().UTC()
		if c.RootID != "" {
			root, err := tx.Find(c.RootID)
			if err != nil {
				return nil, err
			}
			root.ReplyCount--
			if err := tx.Save(root); err != nil {
				return nil, err
			}
		}
		if err := tx.AddTotal(c.TrackID, -1); err != nil {
			return nil, err
		}
	case "reaction":
		if c.Deleted {
			return nil, ErrNotFound
		}
		previous, err := tx.Reaction(c.ID, m.UserID)
		if err != nil {
			return nil, err
		}
		if previous == m.Reaction {
			return c, nil
		}
		if previous == 1 {
			c.LikeCount--
		}
		if previous == -1 {
			c.DislikeCount--
		}
		if m.Reaction == 1 {
			c.LikeCount++
		}
		if m.Reaction == -1 {
			c.DislikeCount++
		}
		if err := tx.SetReaction(c.ID, m.UserID, m.Reaction); err != nil {
			return nil, err
		}
	default:
		return nil, ErrForbidden
	}
	if err := tx.Save(c); err != nil {
		return nil, err
	}
	return c, nil
}
