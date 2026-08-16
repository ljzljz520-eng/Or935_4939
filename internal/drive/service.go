package drive

import (
	"fmt"
	"io"
)

type ReviewService struct {
	store *MemoryStore
}

func NewReviewService(store *MemoryStore) *ReviewService {
	return &ReviewService{store: store}
}

func (s *ReviewService) readDocument(id string) (string, error) {
	handle, err := s.store.OpenDocument(id)
	if err != nil {
		return "", fmt.Errorf("review %s: %w", id, err)
	}
	defer handle.Close()

	content, err := io.ReadAll(handle)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", id, err)
	}
	return string(content), nil
}

func (s *ReviewService) BatchReview(actor string, ids []string, note string) (BatchReviewResult, error) {
	result := BatchReviewResult{Documents: make([]ReviewedDocument, 0, len(ids))}
	if err := s.store.ValidateBatch(actor, ids, note); err != nil {
		return result, err
	}
	for _, id := range ids {
		doc, err := s.store.GetDocument(id)
		if err != nil {
			return result, err
		}
		content, err := s.readDocument(id)
		if err != nil {
			return result, err
		}
		version, err := s.store.saveVersion(actor, id, note, content)
		if err != nil {
			return result, err
		}
		result.Reviewed++
		result.Documents = append(result.Documents, ReviewedDocument{ID: doc.ID, Name: doc.Name, Version: version.Number})
	}
	return result, nil
}
