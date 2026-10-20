package repository

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"strings"
	"sync"
	"testing"
	"time"
)

var commentTestMySQL = flag.String("comment-mysql-dsn", "", "Optional isolated MySQL DSN for comment integration tests")
var commentTestMongo = flag.String("comment-mongo-uri", "", "Optional isolated Mongo replica-set URI for comment integration tests")

func TestCommentPersistence(t *testing.T) {
	t.Run("memory", func(t *testing.T) { exerciseCommentRepository(t, NewMemoryCommentRepository()) })
	if dsn := *commentTestMySQL; dsn != "" {
		t.Run("mysql", func(t *testing.T) {
			db, err := sql.Open("mysql", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, statement := range strings.Split(commentSchema, ";") {
				if strings.TrimSpace(statement) != "" {
					if _, err := db.Exec(statement); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := db.Exec(`ALTER TABLE track_comment_reactions DROP INDEX idx_comment_likers`); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := ensureMySQLCommentLikerIndex(context.Background(), db); err != nil {
					t.Fatal(err)
				}
			}
			exerciseCommentRepository(t, NewMySQLCommentRepository(db))
		})
	}
	if uri := *commentTestMongo; uri != "" {
		t.Run("mongo", func(t *testing.T) {
			ctx := context.Background()
			client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Disconnect(ctx)
			db := client.Database(fmt.Sprintf("comments_qa_%d", time.Now().UnixNano()))
			defer db.Drop(ctx)
			exerciseCommentRepository(t, NewMongoCommentRepository(db))
		})
	}
}
func exerciseCommentRepository(t *testing.T, repo CommentRepository) {
	t.Helper()
	ctx := context.Background()
	prefix := fmt.Sprintf("qa-%d-", time.Now().UnixNano())
	track := prefix + "track"
	stamp := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	create := func(id, client, body, target string, user int64) (*models.TrackComment, error) {
		c := &models.TrackComment{ID: prefix + id, TrackID: track, UserID: user, ClientID: prefix + client, RequestHash: body, Content: body, ReplyToID: target, CreatedAt: stamp, UpdatedAt: stamp}
		return repo.Apply(ctx, CommentMutation{Action: "create", TrackID: track, UserID: user, Comment: c})
	}
	root, err := create("root", "client", "root", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := create(fmt.Sprintf("duplicate%d", i), "client", "root", "", 1)
			if err != nil {
				t.Error(err)
			} else if c.ID != root.ID {
				t.Error("duplicate id")
			}
		}(i)
	}
	wg.Wait()
	counts, err := repo.Totals(ctx, []string{track, prefix + "missing"})
	if err != nil || counts[track] != 1 || counts[prefix+"missing"] != 0 {
		t.Fatalf("batch totals: %v %v", counts, err)
	}
	count, err := repo.Total(ctx, track)
	if err != nil || count != 1 {
		t.Fatalf("retry count=%d %v", count, err)
	}
	author, err := create("author", "author-client", "author reply", root.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := create("reply", "reply-client", "quoted reply", author.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	root, err = repo.Find(ctx, root.ID)
	if err != nil || root.ReplyCount != 2 {
		t.Fatalf("root replies=%+v %v", root, err)
	}
	preview, err := repo.AuthorReplies(ctx, 2, []string{root.ID})
	if err != nil || preview[root.ID] == nil || preview[root.ID].ID != author.ID {
		t.Fatalf("author preview=%v %v", preview, err)
	}
	page, err := repo.List(ctx, track, root.ID, nil, 1)
	if err != nil || len(page) != 1 {
		t.Fatal(err)
	}
	next, err := repo.List(ctx, track, root.ID, &models.CommentCursor{TrackID: track, RootID: root.ID, CreatedAt: page[0].CreatedAt, ID: page[0].ID}, 1)
	if err != nil || len(next) != 1 || next[0].ID == page[0].ID {
		t.Fatalf("tie pagination failed: %v %v", next, err)
	}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 2; n++ {
				if _, err := repo.Apply(ctx, CommentMutation{Action: "reaction", TrackID: track, UserID: int64(100 + i), CommentID: root.ID, Reaction: 1}); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	root, err = repo.Find(ctx, root.ID)
	if err != nil || root.LikeCount != 16 {
		t.Fatalf("like count=%+v %v", root, err)
	}
	root, err = repo.Apply(ctx, CommentMutation{Action: "reaction", TrackID: track, UserID: 100, CommentID: root.ID, Reaction: -1})
	if err != nil || root.LikeCount != 15 || root.DislikeCount != 1 {
		t.Fatalf("switch=%+v %v", root, err)
	}
	reactions, err := repo.Reactions(ctx, 100, []string{root.ID, author.ID})
	if err != nil || reactions[root.ID] != -1 {
		t.Fatal("viewer reaction missing", err)
	}
	liked, err := repo.Likers(ctx, root.ID, 0, 7)
	if err != nil || len(liked) != 7 || liked[0] != 115 || liked[6] != 109 {
		t.Fatalf("liker page: %v %v", liked, err)
	}
	older, err := repo.Likers(ctx, root.ID, liked[len(liked)-1], 20)
	if err != nil || len(older) != 8 || older[0] != 108 || older[7] != 101 {
		t.Fatalf("liker cursor/filter: %v %v", older, err)
	}
	for _, change := range []struct {
		user  int64
		value int
	}{{1, 1}, {101, 0}, {102, -1}} {
		if _, err := repo.Apply(ctx, CommentMutation{Action: "reaction", TrackID: track, UserID: change.user, CommentID: root.ID, Reaction: change.value}); err != nil {
			t.Fatal(err)
		}
	}
	liked, err = repo.Likers(ctx, root.ID, 0, 50)
	if err != nil || len(liked) != 14 || liked[len(liked)-1] != 1 {
		t.Fatalf("self like/cancel/switch: %v %v", liked, err)
	}
	for _, id := range liked {
		if id == 100 || id == 101 || id == 102 {
			t.Fatalf("non-like leaked: %d", id)
		}
	}
	if other, err := repo.Likers(ctx, author.ID, 0, 20); err != nil || len(other) != 0 {
		t.Fatalf("cross-comment likes: %v %v", other, err)
	}
	for _, c := range []*models.TrackComment{author, root} {
		if _, err := repo.Apply(ctx, CommentMutation{Action: "delete", TrackID: track, UserID: c.UserID, CommentID: c.ID}); err != nil {
			t.Fatal(err)
		}
	}
	count, err = repo.Total(ctx, track)
	if err != nil || count != 1 {
		t.Fatalf("deleted count=%d %v", count, err)
	}
	page, err = repo.List(ctx, track, "", nil, 20)
	if err != nil || len(page) != 1 || !page[0].Deleted || page[0].ReplyCount != 1 {
		t.Fatal("missing tombstone", err)
	}
	quote, err := repo.FindMany(ctx, []string{author.ID})
	if err != nil || quote[author.ID].Content != "" || !quote[author.ID].Deleted {
		t.Fatal("deleted text retained", err)
	}
	image := &models.CommentImage{ID: prefix + "image", TrackID: track, UserID: 1, ObjectKey: "reserved.jpg", OSSURL: "https://example.test/reserved.jpg", Width: 640, Height: 480, ByteSize: 1000, CreatedAt: time.Now().UTC()}
	if err := repo.PutImage(ctx, image); err != nil {
		t.Fatal(err)
	}
	c := &models.TrackComment{ID: prefix + "image-comment", TrackID: track, UserID: 1, ClientID: prefix + "img-client", RequestHash: "img", ImageIDs: []string{image.ID}, CreatedAt: stamp, UpdatedAt: stamp}
	saved, err := repo.Apply(ctx, CommentMutation{Action: "create", TrackID: track, UserID: 1, Comment: c})
	if err != nil || len(saved.Images) != 1 {
		t.Fatalf("image metadata missing %v", err)
	}
	if _, err := repo.Apply(ctx, CommentMutation{Action: "create", TrackID: track, UserID: 2, Comment: &models.TrackComment{ID: prefix + "invalid-image", TrackID: track, UserID: 2, ClientID: prefix + "invalid-client", ImageIDs: []string{image.ID}, CreatedAt: stamp, UpdatedAt: stamp}}); err == nil {
		t.Fatal("another user's bound image accepted")
	}
	count, err = repo.Total(ctx, track)
	if err != nil || count != 2 {
		t.Fatalf("rejected mutation changed total=%d %v", count, err)
	}
	if _, err := repo.Apply(ctx, CommentMutation{Action: "delete", TrackID: track, UserID: reply.UserID, CommentID: reply.ID}); err != nil {
		t.Fatal(err)
	}
	page, err = repo.List(ctx, track, "", nil, 20)
	if err != nil || len(page) != 1 || page[0].ID != saved.ID {
		t.Fatal("empty deleted root not hidden", err)
	}
}
