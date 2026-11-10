package models

import (
	"encoding/json"
	"time"
)

// MessageRecord keeps queryable routing fields separate from private, versioned payloads.
type MessageRecord struct {
	AtMicros  int64           `json:"-" bson:"at_micros"`
	ID        string          `json:"id" bson:"_id"`
	Kind      string          `json:"kind" bson:"kind"`
	UserID    int64           `json:"user_id,string" bson:"user_id"`
	PeerID    int64           `json:"peer_id,string" bson:"peer_id"`
	ThreadID  string          `json:"thread_id" bson:"thread_id"`
	Seq       int64           `json:"seq" bson:"seq"`
	CreatedAt time.Time       `json:"created_at" bson:"created_at"`
	Active    bool            `json:"active" bson:"active"`
	Pending   bool            `json:"-" bson:"pending"`
	FlagA     bool            `json:"-" bson:"flag_a"`
	FlagB     bool            `json:"-" bson:"flag_b"`
	Targets   []string        `json:"targets" bson:"targets"`
	Revision  int64           `json:"revision" bson:"revision"`
	Payload   json.RawMessage `json:"payload" bson:"payload"`
}

type MessageUser struct {
	ID        int64  `json:"id,string"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}
type MessageTrack struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PreviewURL string `json:"preview_url"`
	Available  bool   `json:"available"`
}
type DirectMessage struct {
	ID          string        `json:"id"`
	ClientID    string        `json:"client_id"`
	ThreadID    string        `json:"thread_id"`
	Seq         int64         `json:"seq"`
	SenderID    int64         `json:"sender_id,string"`
	ReceiverID  int64         `json:"receiver_id,string"`
	Text        string        `json:"text"`
	TrackID     string        `json:"track_id,omitempty"`
	Track       *MessageTrack `json:"track,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	RequestHash string        `json:"-"`
}
type MessageThread struct {
	A         int64          `json:"a"`
	B         int64          `json:"b"`
	Initiator int64          `json:"initiator"`
	Replied   bool           `json:"replied"`
	ReadA     int64          `json:"read_a"`
	ReadB     int64          `json:"read_b"`
	BlockedA  *time.Time     `json:"blocked_a,omitempty"`
	BlockedB  *time.Time     `json:"blocked_b,omitempty"`
	Last      *DirectMessage `json:"last,omitempty"`
	// A committed journal entry is flushed idempotently before accepting the next send.
	Pending *StoredDirectMessage `json:"pending,omitempty"`
}
type StoredDirectMessage struct {
	Message     DirectMessage `json:"message"`
	RequestHash string        `json:"request_hash"`
}
type MessageConversation struct {
	ReadSeq   int64          `json:"read_seq"`
	Peer      MessageUser    `json:"peer"`
	Last      *DirectMessage `json:"last,omitempty"`
	Unread    int64          `json:"unread"`
	Blocked   bool           `json:"blocked"`
	BlockedAt *time.Time     `json:"blocked_at,omitempty"`
	CanSend   bool           `json:"can_send"`
	SendHint  string         `json:"send_hint,omitempty"`
}
type InboxInteraction struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	ActorID   int64         `json:"actor_id,string"`
	Actor     MessageUser   `json:"actor"`
	TrackID   string        `json:"track_id,omitempty"`
	Track     *MessageTrack `json:"track,omitempty"`
	CommentID string        `json:"comment_id,omitempty"`
	Text      string        `json:"text"`
	CreatedAt time.Time     `json:"created_at"`
	Unread    bool          `json:"unread"`
}
type Announcement struct {
	ID            string     `json:"id"`
	Revision      int64      `json:"revision"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	H5URL         string     `json:"h5_url"`
	Status        string     `json:"status"`
	TargetUserIDs []string   `json:"target_user_ids"`
	PublishAt     time.Time  `json:"publish_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	UpdatedBy     string     `json:"updated_by"`
	Unread        bool       `json:"unread"`
}

// App responses exclude operator identity, audience IDs and editable state.
type AnnouncementView struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	H5URL     string     `json:"h5_url"`
	PublishAt time.Time  `json:"publish_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Unread    bool       `json:"unread"`
}
type AnnouncementPreview struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	PublishAt time.Time `json:"publish_at"`
}
type MessagePage[T any] struct {
	Items      []T    `json:"items"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}
type InboxSummary struct {
	System        int64                `json:"system"`
	Private       int64                `json:"private"`
	Comments      int64                `json:"comments"`
	Reactions     int64                `json:"reactions"`
	Interactions  int64                `json:"interactions"`
	Total         int64                `json:"total"`
	SystemLatest  *AnnouncementPreview `json:"system_latest,omitempty"`
	PrivateLatest *MessageConversation `json:"private_latest,omitempty"`
	Cutoff        time.Time            `json:"cutoff"`
}
type MessageUserState struct {
	AllReadAt time.Time `json:"all_read_at"`
}
