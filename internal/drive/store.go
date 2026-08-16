package drive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound       = errors.New("resource not found")
	ErrPermission     = errors.New("permission denied")
	ErrInvalidRequest = errors.New("invalid request")
	ErrRecycled       = errors.New("document is in the recycle bin")
)

type MemoryStore struct {
	mu             sync.Mutex
	folders        map[string]Folder
	documents      map[string]Document
	permissions    map[string]Permission
	auditLinks     map[string]AuditLink
	logs           []LogEntry
	now            time.Time
	maxOpenHandles int
	openHandles    int
}

func NewFixtureStore() *MemoryStore {
	return NewFixtureStoreWithHandleLimit(3)
}

func NewFixtureStoreWithHandleLimit(limit int) *MemoryStore {
	if limit < 1 {
		limit = 1
	}
	now := time.Date(2026, time.January, 15, 9, 0, 0, 0, time.UTC)
	documents := map[string]Document{
		"report-001":      {ID: "report-001", FolderID: "finance", Name: "Q4 consolidated statements", Kind: KindReport, Status: StatusActive, Content: "revenue=120000;expenses=80000"},
		"report-002":      {ID: "report-002", FolderID: "finance", Name: "Accounts receivable aging", Kind: KindReport, Status: StatusActive, Content: "current=90000;overdue=12000"},
		"contract-001":    {ID: "contract-001", FolderID: "finance", Name: "Vendor services renewal", Kind: KindContract, Status: StatusActive, Content: "vendor=Northwind;term=12 months"},
		"invoice-001":     {ID: "invoice-001", FolderID: "finance", Name: "Vendor invoice January", Kind: KindInvoice, Status: StatusActive, Content: "vendor=Northwind;amount=4200"},
		"invoice-002":     {ID: "invoice-002", FolderID: "finance", Name: "Tax invoice February", Kind: KindInvoice, Status: StatusActive, Content: "vendor=Contoso;amount=1800"},
		"report-archived": {ID: "report-archived", FolderID: "finance", Name: "Prior year archive", Kind: KindReport, Status: StatusRecycled, Content: "archive=true"},
	}
	return &MemoryStore{
		folders: map[string]Folder{
			"finance": {ID: "finance", Name: "Finance records"},
		},
		documents: documents,
		permissions: map[string]Permission{
			"manager-1:finance": {Principal: "manager-1", FolderID: "finance", CanRead: true, CanReview: true, CanUpload: true},
			"auditor-1:finance": {Principal: "auditor-1", FolderID: "finance", CanRead: true},
		},
		auditLinks: map[string]AuditLink{
			"audit-link-2026": {ID: "audit-link-2026", FolderID: "finance", Recipient: "audit@example.test", ExpiresAt: time.Date(2026, time.January, 31, 23, 59, 59, 0, time.UTC)},
		},
		now:            now,
		maxOpenHandles: limit,
	}
}

func (s *MemoryStore) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now
}

func (s *MemoryStore) permission(actor, folderID string) (Permission, bool) {
	p, ok := s.permissions[actor+":"+folderID]
	return p, ok
}

func (s *MemoryStore) CanReview(actor, folderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.permission(actor, folderID)
	return ok && p.CanReview
}

func (s *MemoryStore) GetDocument(id string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[id]
	if !ok {
		return Document{}, ErrNotFound
	}
	doc.Versions = append([]DocumentVersion(nil), doc.Versions...)
	return doc, nil
}

func (s *MemoryStore) OpenDocument(id string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[id]
	if !ok {
		return nil, ErrNotFound
	}
	if s.openHandles >= s.maxOpenHandles {
		return nil, fmt.Errorf("file handle limit reached: %d", s.maxOpenHandles)
	}
	s.openHandles++
	return &fixtureFile{store: s, reader: bytes.NewReader([]byte(doc.Content))}, nil
}

func (s *MemoryStore) saveVersion(actor, id, note, content string) (DocumentVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[id]
	if !ok {
		return DocumentVersion{}, ErrNotFound
	}
	if doc.Status != StatusActive {
		return DocumentVersion{}, ErrRecycled
	}
	version := DocumentVersion{
		Number:     len(doc.Versions) + 1,
		Reviewer:   actor,
		Note:       note,
		Content:    content,
		ReviewedAt: s.now,
	}
	doc.Versions = append(doc.Versions, version)
	s.documents[id] = doc
	s.logs = append(s.logs, LogEntry{At: s.now, Actor: actor, Action: "review.version.created", Resource: id})
	return version, nil
}

func (s *MemoryStore) Search(actor, query string, kind DocumentKind) ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]Document, 0)
	for _, doc := range s.documents {
		p, allowed := s.permission(actor, doc.FolderID)
		if !allowed || !p.CanRead || doc.Status != StatusActive {
			continue
		}
		if kind != "" && doc.Kind != kind {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(doc.Name), query) && !strings.Contains(strings.ToLower(doc.Content), query) {
			continue
		}
		doc.Versions = append([]DocumentVersion(nil), doc.Versions...)
		result = append(result, doc)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *MemoryStore) Permission(actor, folderID string) (Permission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.permission(actor, folderID)
	if !ok {
		return Permission{}, ErrPermission
	}
	return p, nil
}

func (s *MemoryStore) UploadDocument(actor string, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(doc.ID) == "" || strings.TrimSpace(doc.Name) == "" || strings.TrimSpace(doc.Content) == "" {
		return ErrInvalidRequest
	}
	if doc.Kind != KindReport && doc.Kind != KindContract && doc.Kind != KindInvoice {
		return fmt.Errorf("%w: unsupported document kind", ErrInvalidRequest)
	}
	if _, ok := s.folders[doc.FolderID]; !ok {
		return ErrNotFound
	}
	p, allowed := s.permission(actor, doc.FolderID)
	if !allowed || !p.CanUpload {
		return ErrPermission
	}
	if _, exists := s.documents[doc.ID]; exists {
		return fmt.Errorf("%w: document already exists", ErrInvalidRequest)
	}
	doc.Status = StatusActive
	doc.Versions = nil
	s.documents[doc.ID] = doc
	s.logs = append(s.logs, LogEntry{At: s.now, Actor: actor, Action: "document.uploaded", Resource: doc.ID})
	return nil
}

func (s *MemoryStore) RecycledDocuments(actor string) ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Document, 0)
	for _, doc := range s.documents {
		p, allowed := s.permission(actor, doc.FolderID)
		if !allowed || !p.CanRead || doc.Status != StatusRecycled {
			continue
		}
		doc.Versions = append([]DocumentVersion(nil), doc.Versions...)
		result = append(result, doc)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *MemoryStore) RestoreFromRecycleBin(actor, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[id]
	if !ok {
		return ErrNotFound
	}
	p, allowed := s.permission(actor, doc.FolderID)
	if !allowed || !p.CanReview {
		return ErrPermission
	}
	if doc.Status != StatusRecycled {
		return ErrInvalidRequest
	}
	doc.Status = StatusActive
	s.documents[id] = doc
	s.logs = append(s.logs, LogEntry{At: s.now, Actor: actor, Action: "document.restored", Resource: id})
	return nil
}

func (s *MemoryStore) MoveToRecycleBin(actor, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[id]
	if !ok {
		return ErrNotFound
	}
	p, allowed := s.permission(actor, doc.FolderID)
	if !allowed || !p.CanReview {
		return ErrPermission
	}
	if doc.Status == StatusRecycled {
		return ErrRecycled
	}
	doc.Status = StatusRecycled
	s.documents[id] = doc
	s.logs = append(s.logs, LogEntry{At: s.now, Actor: actor, Action: "document.recycled", Resource: id})
	return nil
}

func (s *MemoryStore) ValidateBatch(actor string, ids []string, note string) error {
	if strings.TrimSpace(actor) == "" || len(ids) == 0 || strings.TrimSpace(note) == "" {
		return ErrInvalidRequest
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate document %s", ErrInvalidRequest, id)
		}
		seen[id] = struct{}{}
		doc, err := s.GetDocument(id)
		if err != nil {
			return err
		}
		if doc.Status != StatusActive {
			return ErrRecycled
		}
		if !s.CanReview(actor, doc.FolderID) {
			return ErrPermission
		}
	}
	return nil
}

func (s *MemoryStore) AuditLinkStatus(id string, at time.Time) (AuditLink, AuditLinkState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	link, ok := s.auditLinks[id]
	if !ok {
		return AuditLink{}, "", ErrNotFound
	}
	if link.Revoked {
		return link, AuditLinkRevoked, nil
	}
	if !at.Before(link.ExpiresAt) {
		return link, AuditLinkExpired, nil
	}
	return link, AuditLinkActive, nil
}

func (s *MemoryStore) Logs(actor string) ([]LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.permissions[actor+":finance"]; !ok {
		return nil, ErrPermission
	}
	logs := append([]LogEntry(nil), s.logs...)
	return logs, nil
}

type fixtureFile struct {
	store  *MemoryStore
	reader *bytes.Reader
	closed bool
}

func (f *fixtureFile) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *fixtureFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	f.store.mu.Lock()
	if f.store.openHandles > 0 {
		f.store.openHandles--
	}
	f.store.mu.Unlock()
	return nil
}
