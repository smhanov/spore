package blog

import (
	"context"
	"time"
)

// SitemapEntry represents a single URL entry for use in an XML sitemap.
// Host applications can call Handler.SitemapEntries to retrieve these and
// merge them into their own sitemap.xml output.
type SitemapEntry struct {
	// Loc is the absolute URL of the page.
	Loc string
	// LastMod is the last modification time, if known.
	LastMod *time.Time
}

// SitemapEntries returns sitemap entries for all published blog posts plus
// the blog index page. The host application can merge these into its own
// sitemap.xml. SiteURL must be set in Config for absolute URLs to be generated;
// if it is empty the entries will use relative paths.
func (h *Handler) SitemapEntries(ctx context.Context) ([]SitemapEntry, error) {
	svc := h.svc

	// Collect all published posts (page through in batches of 100).
	var allPosts []Post
	offset := 0
	for {
		batch, err := svc.store.ListPublishedPosts(ctx, 100, offset)
		if err != nil {
			return nil, err
		}
		allPosts = append(allPosts, batch...)
		if len(batch) < 100 {
			break
		}
		offset += len(batch)
	}

	entries := make([]SitemapEntry, 0, len(allPosts)+1)

	// Blog index page.
	entries = append(entries, SitemapEntry{
		Loc: svc.canonicalURL("/"),
	})

	// One entry per published post.
	for _, p := range allPosts {
		lastMod := p.UpdatedAt
		if lastMod == nil {
			lastMod = p.PublishedAt
		}
		entries = append(entries, SitemapEntry{
			Loc:     svc.canonicalURL("/" + p.Slug),
			LastMod: lastMod,
		})
	}

	return entries, nil
}
