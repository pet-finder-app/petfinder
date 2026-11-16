package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
)

type uploadRequest struct {
	Filename    string `json:"filename" minLength:"1" maxLength:"200"`
	ContentType string `json:"content_type" enum:"image/jpeg,image/png,image/webp,application/pdf"`
	ByteSize    int    `json:"byte_size" minimum:"1" maximum:"10485760"`
	Purpose     string `json:"purpose" enum:"avatar,animal,post,document,followup,surrender"`
}
type uploadResponse struct {
	ID        string    `json:"id" format:"uuid"`
	UploadURL string    `json:"upload_url"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func registerMediaRoutes(s *Server, reg *routeRegistry) {
	registerRoute[uploadRequest, uploadResponse](reg, "POST", "/v1/media", "initiateMediaUpload", "Initiate a size-limited binary upload", "Media", 201, s.protected(s.initiateUpload), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "PUT", "/v1/media/{mediaId}/content", "uploadMediaContent", "Upload binary file bytes", "Media", 204, s.protected(s.uploadContent), bearerAuth(false), binaryUploadContract)
	registerRoute[noBody, noBody](reg, "GET", "/v1/media/{mediaId}", "downloadMedia", "Download authorized media bytes", "Media", 200, s.optionalAuth(s.downloadMedia), bearerAuth(true), binaryDownloadContract)
}
func mediaURL(r *http.Request, id string) string {
	base := strings.TrimRight(os.Getenv("PUBLIC_API_URL"), "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/v1/media/" + id
}
func (s *Server) initiateUpload(w http.ResponseWriter, r *http.Request) {
	var req uploadRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ContentType == "application/pdf" && req.Purpose != "document" && req.Purpose != "surrender" {
		writeServiceError(w, ErrInvalid)
		return
	}
	actor, _ := subjectUUID(r)
	var id string
	var created time.Time
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	if _, err = tx.Exec(r.Context(), "SELECT id FROM users WHERE id=$1 FOR UPDATE", actor); err != nil {
		handleDBError(w, err)
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO media_uploads(owner_id,filename,content_type,byte_size,purpose) SELECT $1::uuid,$2::text,$3::text,$4::integer,$5::text WHERE (SELECT COALESCE(sum(byte_size),0) FROM media_uploads WHERE owner_id=$1 AND created_at>now()-interval '1 day')+$4::integer<=104857600 RETURNING id::text,created_at`, actor, filepath.Base(req.Filename), req.ContentType, req.ByteSize, req.Purpose).Scan(&id, &created)
	if err == pgx.ErrNoRows {
		writeProblem(w, 429, "Upload quota exceeded", "the daily upload limit is 100 MiB", nil)
		return
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, uploadResponse{ID: id, UploadURL: "/v1/media/" + id + "/content", URL: mediaURL(r, id), ExpiresAt: created.Add(time.Hour)})
}
func (s *Server) uploadContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "mediaId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var size int
	var mime string
	err := s.pool.QueryRow(r.Context(), "SELECT byte_size,content_type FROM media_uploads WHERE id=$1 AND owner_id=$2 AND uploaded_at IS NULL AND created_at>now()-interval '1 hour'", id, actor).Scan(&size, &mime)
	if err != nil {
		handleDBError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(size)+1)
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) != size || http.DetectContentType(body) != mime {
		writeProblem(w, 422, "Invalid file", "file length or detected content type does not match the upload", nil)
		return
	}
	tag, err := s.pool.Exec(r.Context(), "UPDATE media_uploads SET content=$3,uploaded_at=now() WHERE id=$1 AND owner_id=$2 AND uploaded_at IS NULL", id, actor, body)
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
func (s *Server) downloadMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "mediaId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var body []byte
	var mime, purpose, filename string
	var own, public bool
	err := s.pool.QueryRow(r.Context(), `SELECT m.content,m.content_type,m.purpose,m.filename,COALESCE(m.owner_id=$2,false),
 EXISTS(SELECT 1 FROM posts p JOIN organizations o ON o.id=p.organization_id WHERE m.id=ANY(p.media_ids) AND p.deleted_at IS NULL AND o.verification_status='verified' AND o.deactivated_at IS NULL)
 OR (m.purpose='animal' AND EXISTS(SELECT 1 FROM animal_images i JOIN animals a ON a.id=i.animal_id JOIN organizations o ON o.id=a.responsible_organization_id WHERE i.storage_key LIKE '%/v1/media/'||m.id::text AND i.removed_at IS NULL AND i.moderation_status='approved' AND a.deleted_at IS NULL AND a.status='available' AND o.verification_status='verified'))
 FROM media_uploads m WHERE m.id=$1 AND m.uploaded_at IS NOT NULL`, id, actor).Scan(&body, &mime, &purpose, &filename, &own, &public)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !own && !public && !hasRole(r, auth.RoleAdmin) {
		var reviewer bool
		err = s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM verification_submissions v JOIN media_uploads owned ON owned.owner_id=v.submitted_by AND owned.id=$2::uuid JOIN organization_members om ON om.organization_id=v.organization_id WHERE om.user_id=$1 AND om.status='active' AND om.can_manage_members AND v.evidence::text LIKE '%'||$2::text||'%')`, actor, uuidString(id)).Scan(&reviewer)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if !reviewer {
			writeServiceError(w, ErrNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, no-store")
	if purpose == "document" || mime == "application/pdf" {
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.WriteHeader(200)
	_, _ = w.Write(body)
}

func binaryUploadContract(_ *routeRegistry, op *huma.Operation) {
	op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/octet-stream": {Schema: &huma.Schema{Type: "string", Format: "binary"}}}}
}
func binaryDownloadContract(_ *routeRegistry, op *huma.Operation) {
	op.Responses["200"].Content = map[string]*huma.MediaType{"application/octet-stream": {Schema: &huma.Schema{Type: "string", Format: "binary"}}}
}
