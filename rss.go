package blog

import (
	"encoding/xml"
	"net/http"
	"time"
)

// rssXML is the top-level RSS 2.0 document.
type rssXML struct {
	XMLName   xml.Name   `xml:"rss"`
	Version   string     `xml:"version,attr"`
	AtomNS    string     `xml:"xmlns:atom,attr"`
	ContentNS string     `xml:"xmlns:content,attr"`
	Channel   rssChannel `xml:"channel"`
}

// rssChannel holds the feed metadata and items.
type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language,omitempty"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	AtomLink      atomLink  `xml:"atom:link"`
	Items         []rssItem `xml:"item"`
}

// atomLink provides the self-referencing link required by best practices.
type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// rssItem represents a single entry in the feed.
type rssItem struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	Description    string   `xml:"description"`
	ContentEncoded string   `xml:"content:encoded"`
	PubDate        string   `xml:"pubDate,omitempty"`
	GUID           rssGUID  `xml:"guid"`
	Categories     []string `xml:"category,omitempty"`
}

// rssGUID is a globally unique identifier for an item.
type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func (s *service) handleRSSFeed(w http.ResponseWriter, r *http.Request) {
	posts, err := s.store.ListPublishedPosts(r.Context(), 20, 0)
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}

	// Load tags for all posts
	if len(posts) > 0 {
		_ = s.store.LoadPostsTags(r.Context(), posts)
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}

	title := s.effectiveTitle(settings)
	if title == "" {
		title = "Blog"
	}
	description := s.effectiveDescription(settings)

	siteURL := s.cfg.SiteURL
	if siteURL == "" {
		// Derive from request if not configured
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		siteURL = scheme + "://" + r.Host
	}

	feedURL := s.canonicalURL("/feed")
	if feedURL == "" {
		feedURL = siteURL + s.routePrefix + "/feed"
	}

	var items []rssItem
	var lastBuild time.Time

	for _, p := range posts {
		link := s.canonicalURL("/" + p.Slug)
		if link == "" {
			link = siteURL + s.routePrefix + "/" + p.Slug
		}

		item := rssItem{
			Title:          p.Title,
			Link:           link,
			Description:    p.MetaDescription,
			ContentEncoded: p.ContentHTML,
			GUID: rssGUID{
				IsPermaLink: "true",
				Value:       link,
			},
		}

		if p.PublishedAt != nil {
			item.PubDate = p.PublishedAt.UTC().Format(time.RFC1123Z)
			if p.PublishedAt.After(lastBuild) {
				lastBuild = *p.PublishedAt
			}
		}

		for _, tag := range p.Tags {
			item.Categories = append(item.Categories, tag.Name)
		}

		items = append(items, item)
	}

	lang := s.cfg.SiteLanguage
	if lang == "" {
		lang = "en"
	}

	feed := rssXML{
		Version:   "2.0",
		AtomNS:    "http://www.w3.org/2005/Atom",
		ContentNS: "http://purl.org/rss/1.0/modules/content/",
		Channel: rssChannel{
			Title:       title,
			Link:        siteURL + s.routePrefix + "/",
			Description: description,
			Language:    lang,
			AtomLink: atomLink{
				Href: feedURL,
				Rel:  "self",
				Type: "application/rss+xml",
			},
			Items: items,
		},
	}

	if !lastBuild.IsZero() {
		feed.Channel.LastBuildDate = lastBuild.UTC().Format(time.RFC1123Z)
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		http.Error(w, "failed to encode RSS", http.StatusInternalServerError)
	}
}
