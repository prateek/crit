package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Every endpoint that mutates a comment must fan out comments-changed. The
// watcher cannot cover for a missed one: it suppresses the daemon's own writes,
// so a silent handler leaves other tabs — and a CLI blocked on
// /api/wait-for-event — stuck until an unrelated mutation arrives.
func TestCommentWritesFanOutSSE(t *testing.T) {
	type request struct {
		method string
		target func(fileID, reviewID, fileReplyID, reviewReplyID string) string
		body   string
	}

	cases := []struct {
		name string
		req  request
	}{
		{"add line comment", request{
			http.MethodPost,
			func(_, _, _, _ string) string { return "/api/file/comments?path=test.md" },
			`{"start_line":1,"end_line":1,"body":"b","author":"a"}`,
		}},
		{"add file-scope comment", request{
			http.MethodPost,
			func(_, _, _, _ string) string { return "/api/file/comments?path=test.md" },
			`{"scope":"file","body":"b","author":"a"}`,
		}},
		{"edit line comment", request{
			http.MethodPut,
			func(f, _, _, _ string) string { return "/api/comment/" + f + "?path=test.md" },
			`{"body":"edited"}`,
		}},
		{"delete line comment", request{
			http.MethodDelete,
			func(f, _, _, _ string) string { return "/api/comment/" + f + "?path=test.md" },
			"",
		}},
		{"resolve line comment", request{
			http.MethodPut,
			func(f, _, _, _ string) string { return "/api/comment/" + f + "/resolve?path=test.md" },
			`{"resolved":true}`,
		}},
		{"add reply", request{
			http.MethodPost,
			func(f, _, _, _ string) string { return "/api/comment/" + f + "/replies?path=test.md" },
			`{"body":"r","author":"a"}`,
		}},
		{"edit reply", request{
			http.MethodPut,
			func(f, _, fr, _ string) string {
				return "/api/comment/" + f + "/replies/" + fr + "?path=test.md"
			},
			`{"body":"edited"}`,
		}},
		{"delete reply", request{
			http.MethodDelete,
			func(f, _, fr, _ string) string {
				return "/api/comment/" + f + "/replies/" + fr + "?path=test.md"
			},
			"",
		}},
		{"add review comment", request{
			http.MethodPost,
			func(_, _, _, _ string) string { return "/api/comments" },
			`{"body":"b","author":"a"}`,
		}},
		{"edit review comment", request{
			http.MethodPut,
			func(_, rv, _, _ string) string { return "/api/review-comment/" + rv },
			`{"body":"edited"}`,
		}},
		{"delete review comment", request{
			http.MethodDelete,
			func(_, rv, _, _ string) string { return "/api/review-comment/" + rv },
			"",
		}},
		{"resolve review comment", request{
			http.MethodPut,
			func(_, rv, _, _ string) string { return "/api/review-comment/" + rv + "/resolve" },
			`{"resolved":true}`,
		}},
		{"add review reply", request{
			http.MethodPost,
			func(_, rv, _, _ string) string { return "/api/review-comment/" + rv + "/replies" },
			`{"body":"r","author":"a"}`,
		}},
		{"edit review reply", request{
			http.MethodPut,
			func(_, rv, _, rr string) string {
				return "/api/review-comment/" + rv + "/replies/" + rr
			},
			`{"body":"edited"}`,
		}},
		{"delete review reply", request{
			http.MethodDelete,
			func(_, rv, _, rr string) string {
				return "/api/review-comment/" + rv + "/replies/" + rr
			},
			"",
		}},
		{"clear all comments", request{
			http.MethodDelete,
			func(_, _, _, _ string) string { return "/api/comments" },
			"",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, session := newTestServer(t)

			// Seed directly on the session: the mutators do not fan out on
			// their own, so the subscriber below sees only what the handler
			// under test emits.
			fileComment, ok := session.AddComment("test.md", 1, 1, "new", "seed", "", "seed", "")
			if !ok {
				t.Fatal("seeding a line comment failed")
			}
			fileReply, ok := session.AddReply("test.md", fileComment.ID, "seed", "seed", "")
			if !ok {
				t.Fatal("seeding a reply failed")
			}
			reviewComment := session.AddReviewComment("seed", "seed", "")
			reviewReply, ok := session.AddReviewCommentReply(reviewComment.ID, "seed", "seed", "")
			if !ok {
				t.Fatal("seeding a review reply failed")
			}

			events := session.Subscribe()
			defer session.Unsubscribe(events)

			target := tc.req.target(fileComment.ID, reviewComment.ID, fileReply.ID, reviewReply.ID)
			req := httptest.NewRequest(tc.req.method, target, bytes.NewBufferString(tc.req.body))
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code < 200 || w.Code > 299 {
				t.Fatalf("%s %s = %d, want 2xx: %s", tc.req.method, target, w.Code, w.Body.String())
			}

			select {
			case ev := <-events:
				if ev.Type != "comments-changed" {
					t.Fatalf("event type = %q, want comments-changed", ev.Type)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s %s mutated comments without fanning out comments-changed",
					tc.req.method, target)
			}
		})
	}
}
