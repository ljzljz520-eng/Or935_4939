package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"example.com/accounting-collab-drive/internal/drive"
)

type server struct {
	store   *drive.MemoryStore
	service *drive.ReviewService
}

type batchReviewRequest struct {
	ManagerID   string   `json:"manager_id"`
	DocumentIDs []string `json:"document_ids"`
	Note        string   `json:"note"`
}

func newServer() *server {
	store := drive.NewFixtureStore()
	return &server{store: store, service: drive.NewReviewService(store)}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/api/documents", s.handleDocuments)
	mux.HandleFunc("/api/reviews/batch", s.handleBatchReview)
	mux.HandleFunc("/api/audit-links/", s.handleAuditLink)
	mux.HandleFunc("/api/permissions", s.handlePermission)
	mux.HandleFunc("/api/recycle", s.handleRecycleCollection)
	mux.HandleFunc("/api/recycle/", s.handleRecycle)
	mux.HandleFunc("/api/logs", s.handleLogs)
	return mux
}

func principal(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("X-Principal"))
	if value == "" {
		return "manager-1"
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":      "accounting-collab-drive",
		"fixture_time": s.store.Now().Format(time.RFC3339),
		"endpoints":    []string{"GET|POST /api/documents", "POST /api/reviews/batch", "GET /api/audit-links/{id}", "GET /api/permissions", "GET /api/recycle", "POST /api/recycle/{id}", "POST /api/recycle/{id}/restore", "GET /api/logs"},
	})
}

func (s *server) handleDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var doc drive.Document
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.store.UploadDocument(principal(r), doc); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, doc)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	docs, err := s.store.Search(principal(r), r.URL.Query().Get("q"), drive.DocumentKind(r.URL.Query().Get("kind")))
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (s *server) handlePermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	folderID := r.URL.Query().Get("folder_id")
	permission, err := s.store.Permission(principal(r), folderID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, permission)
}

func (s *server) handleBatchReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request batchReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if request.ManagerID == "" {
		request.ManagerID = principal(r)
	}
	result, err := s.service.BatchReview(request.ManagerID, request.DocumentIDs, request.Note)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"result": result, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) handleAuditLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/audit-links/")
	at := s.store.Now()
	if raw := r.URL.Query().Get("at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid at parameter"})
			return
		}
		at = parsed
	}
	link, state, err := s.store.AuditLinkStatus(id, at)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": link, "state": state})
}

func (s *server) handleRecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/recycle/")
	if strings.HasSuffix(id, "/restore") {
		id = strings.TrimSuffix(id, "/restore")
		if err := s.store.RestoreFromRecycleBin(principal(r), id); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "active", "id": id})
		return
	}
	if err := s.store.MoveToRecycleBin(principal(r), id); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recycled", "id": id})
}

func (s *server) handleRecycleCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	items, err := s.store.RecycledDocuments(principal(r))
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	logs, err := s.store.Logs(principal(r))
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()
	app := newServer()
	log.Printf("accounting collaboration drive listening on %s", *addr)
	if err := http.ListenAndServe(*addr, app.routes()); err != nil {
		fmt.Println(err)
	}
}
