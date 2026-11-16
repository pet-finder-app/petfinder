package api

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"strings"
	"time"
)

type legalDocumentRequest struct {
	Kind        string    `json:"kind" enum:"terms,privacy"`
	Version     string    `json:"version" minLength:"1" maxLength:"60"`
	Title       string    `json:"title" minLength:"1" maxLength:"200"`
	Content     string    `json:"content" minLength:"1" maxLength:"100000"`
	ReviewedBy  string    `json:"reviewed_by" minLength:"2" maxLength:"200"`
	Placeholder bool      `json:"placeholder,omitempty"`
	EffectiveAt time.Time `json:"effective_at"`
}
type legalDocument struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Version     string    `json:"version"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	ReviewedBy  string    `json:"reviewed_by"`
	Placeholder bool      `json:"placeholder"`
	EffectiveAt time.Time `json:"effective_at"`
	PublishedAt time.Time `json:"published_at"`
}
type legalVersions struct {
	Items []legalDocument `json:"items"`
}

func registerLegalRoutes(s *Server, reg *routeRegistry) {
	registerRoute[noBody, legalAcceptanceList](reg, "GET", "/v1/me/legal-acceptances", "listLegalAcceptances", "Read the exact legal versions you accepted", "Legal", 200, s.protected(s.listLegalAcceptances), bearerAuth(false))
	registerRoute[legalAcceptanceRequest, noBody](reg, "POST", "/v1/me/legal-acceptances", "acceptLegalDocument", "Accept a current legal document version", "Legal", 204, s.protected(s.acceptLegalDocument), bearerAuth(false))

	registerRoute[legalDocumentRequest, legalDocument](reg, "POST", "/v1/legal/documents", "publishLegalDocument", "Publish an immutable legal document version", "Legal", 201, s.admin(s.publishLegal), adminOnly)
	registerRoute[noBody, legalDocument](reg, "GET", "/v1/legal/terms", "getTerms", "Read the current terms version", "Legal", 200, http.HandlerFunc(s.currentLegal))
	registerRoute[noBody, legalDocument](reg, "GET", "/v1/legal/privacy", "getPrivacy", "Read the current privacy version", "Legal", 200, http.HandlerFunc(s.currentLegal))
	registerRoute[noBody, legalVersions](reg, "GET", "/v1/legal/terms/versions", "listTermsVersions", "Read effective terms versions", "Legal", 200, http.HandlerFunc(s.legalVersions))
	registerRoute[noBody, legalVersions](reg, "GET", "/v1/legal/privacy/versions", "listPrivacyVersions", "Read effective privacy versions", "Legal", 200, http.HandlerFunc(s.legalVersions))
	registerRoute[noBody, legalDocument](reg, "GET", "/v1/legal/documents/{documentId}", "getLegalVersion", "Read an effective legal document by ID", "Legal", 200, http.HandlerFunc(s.getLegal))
}
func (s *Server) publishLegal(w http.ResponseWriter, r *http.Request) {
	var req legalDocumentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EffectiveAt.IsZero() || strings.TrimSpace(req.Content) == "" {
		writeServiceError(w, ErrInvalid)
		return
	}
	var v legalDocument
	err := s.pool.QueryRow(r.Context(), `INSERT INTO legal_documents(kind,version,title,content,reviewed_by,placeholder,effective_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,kind,version,title,content,reviewed_by,placeholder,effective_at,published_at`, req.Kind, req.Version, req.Title, req.Content, req.ReviewedBy, req.Placeholder, req.EffectiveAt).Scan(&v.ID, &v.Kind, &v.Version, &v.Title, &v.Content, &v.ReviewedBy, &v.Placeholder, &v.EffectiveAt, &v.PublishedAt)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func legalKind(r *http.Request) string {
	if strings.Contains(r.URL.Path, "/terms") {
		return "terms"
	}
	return "privacy"
}
func (s *Server) currentLegal(w http.ResponseWriter, r *http.Request) {
	var v legalDocument
	err := s.pool.QueryRow(r.Context(), `SELECT id::text,kind,version,title,content,reviewed_by,placeholder,effective_at,published_at FROM legal_documents WHERE kind=$1 AND effective_at<=now() ORDER BY effective_at DESC,published_at DESC,id DESC LIMIT 1`, legalKind(r)).Scan(&v.ID, &v.Kind, &v.Version, &v.Title, &v.Content, &v.ReviewedBy, &v.Placeholder, &v.EffectiveAt, &v.PublishedAt)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) legalVersions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), "SELECT id::text,kind,version,title,content,reviewed_by,placeholder,effective_at,published_at FROM legal_documents WHERE kind=$1 AND effective_at<=now() ORDER BY effective_at DESC,published_at DESC,id DESC", legalKind(r))
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []legalDocument{}
	for rows.Next() {
		var v legalDocument
		if err = rows.Scan(&v.ID, &v.Kind, &v.Version, &v.Title, &v.Content, &v.ReviewedBy, &v.Placeholder, &v.EffectiveAt, &v.PublishedAt); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, legalVersions{Items: items})
}
func (s *Server) getLegal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "documentId")
	if !ok {
		invalidPath(w)
		return
	}
	var v legalDocument
	err := s.pool.QueryRow(r.Context(), "SELECT id::text,kind,version,title,content,reviewed_by,placeholder,effective_at,published_at FROM legal_documents WHERE id=$1 AND effective_at<=now()", id).Scan(&v.ID, &v.Kind, &v.Version, &v.Title, &v.Content, &v.ReviewedBy, &v.Placeholder, &v.EffectiveAt, &v.PublishedAt)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

type legalAcceptanceRequest struct {
	DocumentID string `json:"document_id" format:"uuid"`
}
type legalAcceptance struct {
	DocumentID string    `json:"document_id"`
	Kind       string    `json:"kind"`
	Version    string    `json:"version"`
	AcceptedAt time.Time `json:"accepted_at"`
}
type legalAcceptanceList struct {
	Items []legalAcceptance `json:"items"`
}

func (s *Server) recordRegistrationLegal(r *http.Request, tx pgx.Tx, user pgtype.UUID, terms, privacy *string) error {
	for _, entry := range []struct {
		kind string
		id   *string
	}{{"terms", terms}, {"privacy", privacy}} {
		var id pgtype.UUID
		err := tx.QueryRow(r.Context(), "SELECT id FROM legal_documents WHERE kind=$1 AND effective_at<=now() ORDER BY effective_at DESC,published_at DESC,id DESC LIMIT 1", entry.kind).Scan(&id)
		if err != nil {
			return err
		}
		if entry.id != nil && *entry.id != uuidString(id) {
			return ErrConflict
		}
		if _, err = tx.Exec(r.Context(), "INSERT INTO legal_acceptances(user_id,document_id) VALUES($1,$2) ON CONFLICT DO NOTHING", user, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) listLegalAcceptances(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	rows, err := s.pool.Query(r.Context(), "SELECT a.document_id::text,d.kind,d.version,a.accepted_at FROM legal_acceptances a JOIN legal_documents d ON d.id=a.document_id WHERE a.user_id=$1 ORDER BY a.accepted_at,d.kind,d.version", actor)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []legalAcceptance{}
	for rows.Next() {
		var v legalAcceptance
		if err = rows.Scan(&v.DocumentID, &v.Kind, &v.Version, &v.AcceptedAt); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, legalAcceptanceList{Items: items})
}
func (s *Server) acceptLegalDocument(w http.ResponseWriter, r *http.Request) {
	var req legalAcceptanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	tag, err := s.pool.Exec(r.Context(), `INSERT INTO legal_acceptances(user_id,document_id) SELECT $1,d.id FROM legal_documents d WHERE d.id=$2 AND d.effective_at<=now() AND d.id=(SELECT id FROM legal_documents WHERE kind=d.kind AND effective_at<=now() ORDER BY effective_at DESC,published_at DESC,id DESC LIMIT 1) ON CONFLICT(user_id,document_id) DO UPDATE SET document_id=EXCLUDED.document_id`, actor, req.DocumentID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	w.WriteHeader(204)
}
