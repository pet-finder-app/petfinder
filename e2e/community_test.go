//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestCommunityAndReadState(t *testing.T) {
	s := newAdoptionSetup(t)
	stranger := registerUser(t)
	follow := "/v1/organizations/" + s.org.ID + "/follow"
	s.adopter.do("PUT", follow, nil, 204, nil)
	s.adopter.do("PUT", follow, nil, 204, nil)
	var followers struct {
		FollowerCount int `json:"follower_count"`
	}
	client{t: t}.do("GET", "/v1/organizations/"+s.org.ID+"/followers", nil, 200, &followers)
	if followers.FollowerCount != 1 {
		t.Fatal("follow not idempotent")
	}
	var post struct {
		ID        string
		LikeCount int `json:"like_count"`
	}
	posts := "/v1/organizations/" + s.org.ID + "/posts"
	stranger.do("POST", posts, map[string]any{"body": "News from the shelter"}, 403, nil)
	s.owner.do("POST", posts, map[string]any{"body": "News from the shelter", "tags": []string{"adoption"}}, 201, &post)
	path := "/v1/posts/" + post.ID
	s.adopter.do("PUT", path+"/like", nil, 204, nil)
	s.adopter.do("PUT", path+"/like", nil, 204, nil)
	client{t: t}.do("GET", path, nil, 200, &post)
	if post.LikeCount != 1 {
		t.Fatal("like count incorrect")
	}
	var feed struct{ Items []struct{ ID string } }
	s.adopter.do("GET", "/v1/feed", nil, 200, &feed)
	if len(feed.Items) != 1 || feed.Items[0].ID != post.ID {
		t.Fatal("following feed missing post")
	}
	stranger.do("GET", "/v1/feed", nil, 200, &feed)
	if len(feed.Items) != 0 {
		t.Fatal("feed ignored following")
	}
	s.adopter.do("POST", "/v1/reports", map[string]any{"subject_type": "post", "subject_id": post.ID, "category": "inappropriate_content", "description": "Review the content in this post"}, 201, nil)
	conversation := "/v1/conversations/" + s.conversationID
	var message struct{ Message struct{ ID string } }
	s.owner.do("POST", conversation+"/messages", map[string]any{"body": "Welcome to our shelter"}, 201, &message)
	var read struct {
		UnreadCount int `json:"unread_count"`
	}
	s.adopter.do("GET", conversation+"/read-state", nil, 200, &read)
	if read.UnreadCount != 1 {
		t.Fatal("unread count missing")
	}
	s.adopter.do("GET", conversation+"/messages", nil, 200, nil)
	s.adopter.do("GET", conversation+"/read-state", nil, 200, &read)
	if read.UnreadCount != 1 {
		t.Fatal("listing unexpectedly marked messages read")
	}
	s.adopter.do("PUT", conversation+"/read-state", map[string]any{"message_id": message.Message.ID}, 200, &read)
	if read.UnreadCount != 0 {
		t.Fatal("mark read failed")
	}
	stranger.do("GET", conversation+"/read-state", nil, 403, nil)
	var count struct{ Count int }
	s.adopter.do("GET", "/v1/notifications/unread", nil, 200, &count)
	if count.Count < 1 {
		t.Fatal("notifications missing")
	}
	s.adopter.do("PUT", "/v1/notifications/read", nil, 204, nil)
	s.adopter.do("GET", "/v1/notifications/unread", nil, 200, &count)
	if count.Count != 0 {
		t.Fatal("notifications not marked read")
	}
	interest := "/v1/animals/" + s.animal.ID
	s.owner.do("POST", interest+"/conversations", map[string]any{"user_id": s.adopter.User.ID}, 409, nil)
	s.adopter.do("PATCH", "/v1/me", map[string]any{"profile": map[string]any{"allow_organization_contact": true}}, 200, nil)
	s.owner.do("POST", interest+"/conversations", map[string]any{"user_id": s.adopter.User.ID}, 201, nil)
	stranger.do("GET", interest+"/interest", nil, 403, nil)
	s.owner.do("GET", interest+"/interest", nil, 200, nil)
	s.owner.do("GET", "/v1/organizations/"+s.org.ID+"/analytics", nil, 200, nil)
	s.owner.do("PUT", path, map[string]any{"body": "Updated news"}, 200, nil)
	s.owner.do("DELETE", path, nil, 204, nil)
	client{t: t}.do("GET", path, nil, 404, nil)
}

func TestBinaryMediaAndLifecycle(t *testing.T) {
	u, stranger := registerUser(t), registerUser(t)
	// A small valid PNG header is sufficient to exercise MIME detection and exact lengths.
	data := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0}
	var media struct {
		ID        string
		UploadURL string `json:"upload_url"`
		URL       string
	}
	u.do("POST", "/v1/media", map[string]any{"filename": "animal.png", "content_type": "image/png", "byte_size": len(data), "purpose": "animal"}, 201, &media)
	req, err := http.NewRequest("PUT", baseURL+media.UploadURL, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+u.AccessToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatalf("upload: %d %s", res.StatusCode, body)
	}
	stranger.do("GET", "/v1/media/"+media.ID, nil, 404, nil)
	client{t: t}.do("GET", "/v1/media/"+media.ID, nil, 404, nil)
	req, _ = http.NewRequest("GET", baseURL+"/v1/media/"+media.ID, nil)
	req.Header.Set("Authorization", "Bearer "+u.AccessToken)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !bytes.Equal(body, data) {
		t.Fatal("stored media bytes do not match")
	}
	var a animal
	animalBody := animalRequest("", "City "+unique())
	delete(animalBody, "organization_id")
	animalBody["species"] = "bird"
	animalBody["attributes"] = map[string]any{"height_cm": 25, "vaccinated": true, "temperament": []string{"gentle"}}
	u.do("POST", "/v1/animals", animalBody, 201, &a)
	owner := registerUser(t)
	org := createOrganization(t, owner, "City "+unique())
	verifyOrganization(t, registerAdmin(t), org.ID)
	path := "/v1/animals/" + a.ID
	stranger.do("PUT", path+"/organization", map[string]any{"organization_id": org.ID, "reason": "Please mediate adoption"}, 403, nil)
	u.do("PUT", path+"/organization", map[string]any{"organization_id": org.ID, "reason": "Please mediate adoption"}, 200, nil)
	u.do("POST", path+"/archive", map[string]any{"reason": "No longer seeking adoption"}, 200, nil)
	u.do("GET", path+"/history", nil, 200, nil)
	u.do("DELETE", path, map[string]any{"reason": "Remove the archived listing"}, 204, nil)
	u.do("GET", path, nil, 404, nil)
	s := newAdoptionSetup(t)
	s.owner.do("POST", "/v1/animals/"+s.animal.ID+"/archive", map[string]any{"reason": "Cannot archive with applications"}, 409, nil)
	s.adopter.do("POST", "/v1/applications/"+s.applicationID+"/cancellation", map[string]any{"reason": "Cannot administratively cancel"}, 403, nil)
	s.owner.do("POST", "/v1/applications/"+s.applicationID+"/cancellation", map[string]any{"reason": "Application withdrawn by agreement"}, 200, nil)
	s.owner.do("POST", "/v1/animals/"+s.animal.ID+"/archive", map[string]any{"reason": "Archive after cancellation"}, 200, nil)
}
