package commons

import (
	"diploma/crdt"

	"github.com/google/uuid"
)

type MessageType string

const (
	DocSyncMessage MessageType = "docSync" // syncing documents
	DocReqMessage  MessageType = "docReq"  // requesting documents
	SiteIDMessage  MessageType = "SiteID"  // generating site IDs
	JoinMessage    MessageType = "join"    // joining messages
	UsersMessage   MessageType = "users"   // list of active users
)

type Message struct {
	Username  string        `json:"username"`
	Text      string        `json:"text"`
	Type      MessageType   `json:"type"`
	ID        uuid.UUID     `json:"ID"`
	Operation Operation     `json:"operation"`
	Document  crdt.Document `json:"document"`
}
