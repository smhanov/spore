package blog

import (
	"testing"
	"time"
)

func timePtr(value time.Time) *time.Time {
	return &value
}

func adminSortPost(id string, publishedAt, updatedAt *time.Time) Post {
	return Post{ID: id, Slug: id, Title: id, PublishedAt: publishedAt, UpdatedAt: updatedAt}
}

func postIDList(posts []Post) []string {
	ids := make([]string, 0, len(posts))
	for _, post := range posts {
		ids = append(ids, post.ID)
	}
	return ids
}

func assertAdminOrder(t *testing.T, got []Post, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sorted %d posts, want %d (order: %v)", len(got), len(want), postIDList(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d = %s, want %s (order: %v)", i, got[i].ID, id, postIDList(got))
		}
	}
}

// TestSortPostsForAdminOrdersByModificationTime pins the admin list ordering:
// strict modification-time descending regardless of draft/published status, so
// recently edited drafts interleave with published posts instead of pinning to
// the top, and posts never touched (no UpdatedAt, no PublishedAt) sink last.
func TestSortPostsForAdminOrdersByModificationTime(t *testing.T) {
	jan := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	posts := []Post{
		adminSortPost("draft-edited", nil, timePtr(feb)),
		adminSortPost("published-old", timePtr(jan), timePtr(jan)),
		adminSortPost("draft-never-touched", nil, nil),
		adminSortPost("published-edited", timePtr(jan), timePtr(mar)),
	}

	assertAdminOrder(t, sortPostsForAdmin(posts), []string{
		"published-edited",
		"draft-edited",
		"published-old",
		"draft-never-touched",
	})
}

func TestSortPostsForAdminTieBreaksByIDAscending(t *testing.T) {
	stamp := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	posts := []Post{
		adminSortPost("b-published-tie", timePtr(stamp), timePtr(stamp)),
		adminSortPost("a-draft-tie", nil, timePtr(stamp)),
		adminSortPost("d-never-touched", nil, nil),
		adminSortPost("c-never-touched", nil, nil),
	}

	assertAdminOrder(t, sortPostsForAdmin(posts), []string{
		"a-draft-tie",
		"b-published-tie",
		"c-never-touched",
		"d-never-touched",
	})
}
