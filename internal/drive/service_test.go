package drive

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestBatchReviewCreatesAnnotatedVersionsForAllDocuments(t *testing.T) {
	store := NewFixtureStore()
	service := NewReviewService(store)
	ids := []string{"report-001", "report-002", "contract-001", "invoice-001", "invoice-002"}

	result, err := service.BatchReview("manager-1", ids, "confirm source totals")
	if err != nil {
		t.Fatalf("batch review returned an error: %v", err)
	}
	if diff := cmp.Diff(len(ids), result.Reviewed); diff != "" {
		t.Fatalf("reviewed count mismatch (-want +got):\n%s", diff)
	}
	for _, id := range ids {
		doc, getErr := store.GetDocument(id)
		if getErr != nil {
			t.Fatalf("document lookup failed: %v", getErr)
		}
		if diff := cmp.Diff(1, len(doc.Versions)); diff != "" {
			t.Fatalf("version count mismatch for %s (-want +got):\n%s", id, diff)
		}
	}
}

func TestSearchReturnsAccessibleActiveDocuments(t *testing.T) {
	store := NewFixtureStore()
	result, err := store.Search("manager-1", "vendor", KindContract)
	if err != nil {
		t.Fatalf("search returned an error: %v", err)
	}
	want := []string{"contract-001"}
	got := make([]string, 0, len(result))
	for _, doc := range result {
		got = append(got, doc.ID)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("search results mismatch (-want +got):\n%s", diff)
	}
}

func TestRecycledDocumentsStayOutOfSearchResults(t *testing.T) {
	store := NewFixtureStore()
	if err := store.MoveToRecycleBin("manager-1", "invoice-001"); err != nil {
		t.Fatalf("recycle operation failed: %v", err)
	}
	result, err := store.Search("manager-1", "invoice", "")
	if err != nil {
		t.Fatalf("search returned an error: %v", err)
	}
	for _, doc := range result {
		if doc.ID == "invoice-001" {
			t.Fatalf("recycled document appeared in search results")
		}
	}
}

func TestExpiredAuditLinksRejectAccess(t *testing.T) {
	store := NewFixtureStore()
	_, current, err := store.AuditLinkStatus("audit-link-2026", time.Date(2026, time.January, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("current link status failed: %v", err)
	}
	if current != AuditLinkActive {
		t.Fatalf("current link status = %s, want %s", current, AuditLinkActive)
	}
	_, expired, err := store.AuditLinkStatus("audit-link-2026", time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expired link status failed: %v", err)
	}
	if expired != AuditLinkExpired {
		t.Fatalf("expired link status = %s, want %s", expired, AuditLinkExpired)
	}
}

func TestReviewStatusRejectsRecycledDocuments(t *testing.T) {
	store := NewFixtureStore()
	if err := store.MoveToRecycleBin("manager-1", "invoice-001"); err != nil {
		t.Fatalf("recycle operation failed: %v", err)
	}
	service := NewReviewService(store)
	_, err := service.BatchReview("manager-1", []string{"invoice-001"}, "check status")
	if err != ErrRecycled {
		t.Fatalf("review status error = %v, want %v", err, ErrRecycled)
	}
}

func TestLogsRecordBusinessActions(t *testing.T) {
	store := NewFixtureStoreWithHandleLimit(8)
	if err := store.MoveToRecycleBin("manager-1", "invoice-001"); err != nil {
		t.Fatalf("recycle operation failed: %v", err)
	}
	service := NewReviewService(store)
	if _, err := service.BatchReview("manager-1", []string{"report-001"}, "check totals"); err != nil {
		t.Fatalf("review operation failed: %v", err)
	}
	logs, err := store.Logs("manager-1")
	if err != nil {
		t.Fatalf("log query failed: %v", err)
	}
	want := []string{"document.recycled", "review.version.created"}
	got := make([]string, 0, len(logs))
	for _, entry := range logs {
		got = append(got, entry.Action)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("log actions mismatch (-want +got):\n%s", diff)
	}
}

func TestFinanceUploadAddsDocumentToSearch(t *testing.T) {
	store := NewFixtureStore()
	doc := Document{ID: "invoice-003", FolderID: "finance", Name: "March tax invoice", Kind: KindInvoice, Content: "vendor=Fabrikam;amount=2100"}
	if err := store.UploadDocument("manager-1", doc); err != nil {
		t.Fatalf("upload operation failed: %v", err)
	}
	result, err := store.Search("manager-1", "March", KindInvoice)
	if err != nil {
		t.Fatalf("search returned an error: %v", err)
	}
	if diff := cmp.Diff([]string{"invoice-003"}, []string{result[0].ID}); diff != "" {
		t.Fatalf("uploaded document mismatch (-want +got):\n%s", diff)
	}
}

func TestFolderPermissionBlocksReviewByAuditor(t *testing.T) {
	store := NewFixtureStore()
	service := NewReviewService(store)
	_, err := service.BatchReview("auditor-1", []string{"report-001"}, "review source")
	if err != ErrPermission {
		t.Fatalf("review permission error = %v, want %v", err, ErrPermission)
	}
}

func TestRecycleBinRestoresDocuments(t *testing.T) {
	store := NewFixtureStore()
	if err := store.MoveToRecycleBin("manager-1", "invoice-001"); err != nil {
		t.Fatalf("recycle operation failed: %v", err)
	}
	items, err := store.RecycledDocuments("manager-1")
	if err != nil {
		t.Fatalf("recycle bin query failed: %v", err)
	}
	if diff := cmp.Diff([]string{"invoice-001", "report-archived"}, []string{items[0].ID, items[1].ID}); diff != "" {
		t.Fatalf("recycle bin mismatch (-want +got):\n%s", diff)
	}
	if err := store.RestoreFromRecycleBin("manager-1", "invoice-001"); err != nil {
		t.Fatalf("restore operation failed: %v", err)
	}
	result, err := store.Search("manager-1", "January", KindInvoice)
	if err != nil {
		t.Fatalf("restored document search failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "invoice-001" {
		t.Fatalf("restored document was not searchable")
	}
}
