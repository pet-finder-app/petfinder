package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
)

type postRequest struct {
	Body     string   `json:"body" minLength:"1" maxLength:"10000"`
	MediaIDs []string `json:"media_ids,omitempty" maxItems:"20"`
	Tags     []string `json:"tags,omitempty" maxItems:"20"`
}
type postResponse struct {
	ID             string    `json:"id" format:"uuid"`
	OrganizationID string    `json:"organization_id" format:"uuid"`
	AuthorID       string    `json:"author_id" format:"uuid"`
	Body           string    `json:"body"`
	MediaIDs       []string  `json:"media_ids"`
	Tags           []string  `json:"tags"`
	LikeCount      int       `json:"like_count"`
	Liked          bool      `json:"liked"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type countResponse struct {
	Count int `json:"count"`
}
type organizationFollowersResponse struct {
	FollowerCount int  `json:"follower_count"`
	Following     bool `json:"following"`
}

const postProjection = `jsonb_build_object('id',p.id,'organization_id',p.organization_id,'author_id',p.author_id,'body',p.body,'media_ids',p.media_ids,'tags',p.tags,'created_at',p.created_at,'updated_at',p.updated_at,'like_count',(SELECT count(*) FROM post_likes l WHERE l.post_id=p.id),'liked',EXISTS(SELECT 1 FROM post_likes l WHERE l.post_id=p.id AND l.user_id=$1))`

func registerCommunityRoutes(s *Server, reg *routeRegistry) {
	registerRoute[noBody, collectionResponse[postResponse]](reg, "GET", "/v1/posts", "listPosts", "List public organization posts", "Posts", 200, s.optionalAuth(s.listPosts), bearerAuth(true), queryParameters[paginationQuery], queryParameters[organizationIDQuery])
	registerRoute[noBody, collectionResponse[postResponse]](reg, "GET", "/v1/feed", "followingFeed", "List posts from followed organizations", "Posts", 200, s.protected(s.listPosts), bearerAuth(false), queryParameters[paginationQuery])
	registerRoute[postRequest, postResponse](reg, "POST", "/v1/organizations/{organizationId}/posts", "createPost", "Publish an organization post", "Posts", 201, s.protected(s.createPost), bearerAuth(false))
	registerRoute[noBody, postResponse](reg, "GET", "/v1/posts/{postId}", "getPost", "Read a public post", "Posts", 200, s.optionalAuth(s.getPost), bearerAuth(true))
	registerRoute[postRequest, postResponse](reg, "PUT", "/v1/posts/{postId}", "updatePost", "Edit an organization post", "Posts", 200, s.protected(s.updatePost), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "DELETE", "/v1/posts/{postId}", "deletePost", "Retain and remove an organization post", "Posts", 204, s.protected(s.deletePost), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "PUT", "/v1/posts/{postId}/like", "likePost", "Like a post idempotently", "Posts", 204, s.protected(s.likePost), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "DELETE", "/v1/posts/{postId}/like", "unlikePost", "Remove your post like", "Posts", 204, s.protected(s.likePost), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "PUT", "/v1/organizations/{organizationId}/follow", "followOrganization", "Follow a verified organization", "Following", 204, s.protected(s.followOrganization), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "DELETE", "/v1/organizations/{organizationId}/follow", "unfollowOrganization", "Unfollow an organization", "Following", 204, s.protected(s.followOrganization), bearerAuth(false))
	registerRoute[noBody, organizationFollowersResponse](reg, "GET", "/v1/organizations/{organizationId}/followers", "organizationFollowerCount", "Get follower count and your follow state", "Following", 200, s.optionalAuth(s.organizationFollowers), bearerAuth(true))
	registerRoute[noBody, collectionResponse[organizationResponse]](reg, "GET", "/v1/me/following", "listFollowing", "List your followed organizations", "Following", 200, s.protected(s.listFollowing), bearerAuth(false), queryParameters[paginationQuery])
}
func (s *Server) canPublish(r *http.Request, org pgtype.UUID) bool {
	actor, _ := subjectUUID(r)
	member, ok := s.activeMember(r, org, actor)
	return hasRole(r, auth.RoleAdmin) || (ok && member.CanPublishAnimals)
}
func (s *Server) publicOrganization(r *http.Request, org pgtype.UUID) bool {
	o, err := s.queries.GetOrganizationByID(r.Context(), org)
	return err == nil && o.VerificationStatus == "verified" && !o.DeactivatedAt.Valid
}
func (s *Server) checkMedia(w http.ResponseWriter, r *http.Request, ids []string, purpose string) bool {
	actor, _ := subjectUUID(r)
	for _, raw := range ids {
		id, ok := pathUUIDValue(raw)
		if !ok {
			writeServiceError(w, ErrInvalid)
			return false
		}
		var allowed bool
		err := s.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM media_uploads WHERE id=$1 AND owner_id=$2 AND purpose=$3 AND uploaded_at IS NOT NULL)", id, actor, purpose).Scan(&allowed)
		if err != nil {
			handleDBError(w, err)
			return false
		}
		if !allowed {
			writeServiceError(w, ErrForbidden)
			return false
		}
	}
	return true
}
func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	var org pgtype.UUID
	if raw := r.URL.Query().Get("organization_id"); raw != "" {
		var ok bool
		org, ok = pathUUIDValue(raw)
		if !ok {
			invalidPath(w)
			return
		}
	}
	following := r.URL.Path == "/v1/feed"
	rows, err := s.pool.Query(r.Context(), `SELECT `+postProjection+` FROM posts p JOIN organizations o ON o.id=p.organization_id WHERE p.deleted_at IS NULL AND o.verification_status='verified' AND o.deactivated_at IS NULL AND ($2::uuid IS NULL OR p.organization_id=$2) AND (NOT $3::boolean OR EXISTS(SELECT 1 FROM organization_followers f WHERE f.organization_id=p.organization_id AND f.user_id=$1)) ORDER BY p.created_at DESC,p.id LIMIT $4 OFFSET $5`, actor, org, following, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []postResponse{}
	for rows.Next() {
		var p postResponse
		if err = rows.Scan(&p); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, p)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, 200, collectionResponse[postResponse]{Items: items, Page: page})
}
func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "postId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var p postResponse
	err := s.pool.QueryRow(r.Context(), `SELECT `+postProjection+` FROM posts p JOIN organizations o ON o.id=p.organization_id WHERE p.id=$2 AND p.deleted_at IS NULL AND o.verification_status='verified' AND o.deactivated_at IS NULL`, actor, id).Scan(&p)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, p)
}
func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.canPublish(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if !s.publicOrganization(r, org) {
		writeServiceError(w, ErrConflict)
		return
	}
	var req postRequest
	if !decodeJSON(w, r, &req) || !s.checkMedia(w, r, req.MediaIDs, "post") {
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeServiceError(w, ErrInvalid)
		return
	}
	actor, _ := subjectUUID(r)
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO posts(organization_id,author_id,body,media_ids,tags) VALUES($1,$2,$3,COALESCE($4::uuid[],'{}'),COALESCE($5::text[],'{}')) RETURNING id::text`, org, actor, req.Body, req.MediaIDs, req.Tags).Scan(&id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	r.SetPathValue("postId", id)
	var p postResponse
	err = s.pool.QueryRow(r.Context(), `SELECT `+postProjection+` FROM posts p WHERE p.id=$2`, actor, id).Scan(&p)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, p)
}
func (s *Server) postOrganization(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, bool) {
	id, ok := pathUUID(r, "postId")
	if !ok {
		invalidPath(w)
		return id, pgtype.UUID{}, false
	}
	var org pgtype.UUID
	err := s.pool.QueryRow(r.Context(), "SELECT organization_id FROM posts WHERE id=$1 AND deleted_at IS NULL", id).Scan(&org)
	if err != nil {
		handleDBError(w, err)
		return id, org, false
	}
	return id, org, true
}
func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	id, org, ok := s.postOrganization(w, r)
	if !ok {
		return
	}
	if !s.canPublish(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req postRequest
	if !decodeJSON(w, r, &req) || !s.checkMedia(w, r, req.MediaIDs, "post") {
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeServiceError(w, ErrInvalid)
		return
	}
	_, err := s.pool.Exec(r.Context(), `UPDATE posts SET body=$2,media_ids=COALESCE($3::uuid[],'{}'),tags=COALESCE($4::text[],'{}'),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, req.Body, req.MediaIDs, req.Tags)
	if err != nil {
		handleDBError(w, err)
		return
	}
	s.getPost(w, r)
}
func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	id, org, ok := s.postOrganization(w, r)
	if !ok {
		return
	}
	if !s.canPublish(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if _, err := s.pool.Exec(r.Context(), "UPDATE posts SET deleted_at=now() WHERE id=$1", id); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) likePost(w http.ResponseWriter, r *http.Request) {
	id, org, ok := s.postOrganization(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	var err error
	if r.Method == "DELETE" {
		_, err = s.pool.Exec(r.Context(), "DELETE FROM post_likes WHERE post_id=$1 AND user_id=$2", id, actor)
	} else {
		if !s.publicOrganization(r, org) {
			writeServiceError(w, ErrNotFound)
			return
		}
		_, err = s.pool.Exec(r.Context(), "INSERT INTO post_likes(post_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", id, actor)
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) followOrganization(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var err error
	if r.Method == "DELETE" {
		_, err = s.pool.Exec(r.Context(), "DELETE FROM organization_followers WHERE organization_id=$1 AND user_id=$2", org, actor)
	} else {
		if !s.publicOrganization(r, org) {
			writeServiceError(w, ErrNotFound)
			return
		}
		_, err = s.pool.Exec(r.Context(), "INSERT INTO organization_followers(organization_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", org, actor)
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) organizationFollowers(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.publicOrganization(r, org) {
		writeServiceError(w, ErrNotFound)
		return
	}
	actor, _ := subjectUUID(r)
	var v organizationFollowersResponse
	err := s.pool.QueryRow(r.Context(), "SELECT count(*),COALESCE(bool_or(user_id=$2),false) FROM organization_followers WHERE organization_id=$1", org, actor).Scan(&v.FollowerCount, &v.Following)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) listFollowing(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	rows, err := s.pool.Query(r.Context(), `SELECT organization_id FROM organization_followers WHERE user_id=$1 ORDER BY created_at DESC,organization_id LIMIT $2 OFFSET $3`, actor, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	ids := []pgtype.UUID{}
	for rows.Next() {
		var id pgtype.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			handleDBError(w, err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	items := []organizationResponse{}
	for _, id := range ids {
		org, err := s.queries.GetOrganizationByID(r.Context(), id)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if org.DeactivatedAt.Valid {
			continue
		}
		items = append(items, organizationView(org, false))
	}
	page.Total = len(items)
	writeJSON(w, 200, collectionResponse[organizationResponse]{Items: items, Page: page})
}
