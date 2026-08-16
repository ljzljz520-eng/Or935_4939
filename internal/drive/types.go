package drive

import "time"

type DocumentKind string

const (
	KindReport   DocumentKind = "report"
	KindContract DocumentKind = "contract"
	KindInvoice  DocumentKind = "invoice"
)

type DocumentStatus string

const (
	StatusActive   DocumentStatus = "active"
	StatusRecycled DocumentStatus = "recycled"
)

type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Permission struct {
	Principal string `json:"principal"`
	FolderID  string `json:"folder_id"`
	CanRead   bool   `json:"can_read"`
	CanReview bool   `json:"can_review"`
	CanUpload bool   `json:"can_upload"`
}

type DocumentVersion struct {
	Number     int       `json:"number"`
	Reviewer   string    `json:"reviewer"`
	Note       string    `json:"note"`
	Content    string    `json:"content"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

type Document struct {
	ID       string            `json:"id"`
	FolderID string            `json:"folder_id"`
	Name     string            `json:"name"`
	Kind     DocumentKind      `json:"kind"`
	Status   DocumentStatus    `json:"status"`
	Content  string            `json:"content"`
	Versions []DocumentVersion `json:"versions"`
}

type AuditLink struct {
	ID        string    `json:"id"`
	FolderID  string    `json:"folder_id"`
	Recipient string    `json:"recipient"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

type AuditLinkState string

const (
	AuditLinkActive  AuditLinkState = "active"
	AuditLinkExpired AuditLinkState = "expired"
	AuditLinkRevoked AuditLinkState = "revoked"
)

type LogEntry struct {
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Action   string    `json:"action"`
	Resource string    `json:"resource"`
}

type ReviewedDocument struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type BatchReviewResult struct {
	Reviewed  int                `json:"reviewed"`
	Documents []ReviewedDocument `json:"documents"`
}
