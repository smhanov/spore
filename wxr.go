package blog

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	wxrExcerptNS = "http://wordpress.org/export/1.2/excerpt/"
	wxrContentNS = "http://purl.org/rss/1.0/modules/content/"
	wxrWfwNS     = "http://wellformedweb.org/CommentAPI/"
	wxrDCNS      = "http://purl.org/dc/elements/1.1/"
	wxrWPNS      = "http://wordpress.org/export/1.2/"
)

type cdataString string

func (c cdataString) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(struct {
		Text string `xml:",cdata"`
	}{Text: string(c)}, start)
}

type wxrRSS struct {
	XMLName   xml.Name   `xml:"rss"`
	Version   string     `xml:"version,attr"`
	ExcerptNS string     `xml:"xmlns:excerpt,attr"`
	ContentNS string     `xml:"xmlns:content,attr"`
	WfwNS     string     `xml:"xmlns:wfw,attr"`
	DCNS      string     `xml:"xmlns:dc,attr"`
	WPNS      string     `xml:"xmlns:wp,attr"`
	Channel   wxrChannel `xml:"channel"`
}

type wxrChannel struct {
	Title       string      `xml:"title"`
	Link        string      `xml:"link"`
	Description string      `xml:"description"`
	PubDate     string      `xml:"pubDate"`
	Language    string      `xml:"language,omitempty"`
	WXRVersion  string      `xml:"wp:wxr_version"`
	BaseSiteURL string      `xml:"wp:base_site_url"`
	BaseBlogURL string      `xml:"wp:base_blog_url"`
	Authors     []wxrAuthor `xml:"wp:author,omitempty"`
	Tags        []wxrTag    `xml:"wp:tag,omitempty"`
	Items       []wxrItem   `xml:"item"`
}

type wxrAuthor struct {
	AuthorID          int         `xml:"wp:author_id"`
	AuthorLogin       string      `xml:"wp:author_login"`
	AuthorEmail       string      `xml:"wp:author_email"`
	AuthorDisplayName cdataString `xml:"wp:author_display_name"`
	AuthorFirstName   cdataString `xml:"wp:author_first_name,omitempty"`
	AuthorLastName    cdataString `xml:"wp:author_last_name,omitempty"`
}

type wxrTag struct {
	TermID int         `xml:"wp:term_id"`
	Slug   string      `xml:"wp:tag_slug"`
	Name   cdataString `xml:"wp:tag_name"`
}

type wxrGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type wxrItem struct {
	Title          string        `xml:"title"`
	Link           string        `xml:"link"`
	PubDate        string        `xml:"pubDate"`
	Creator        cdataString   `xml:"dc:creator,omitempty"`
	GUID           wxrGUID       `xml:"guid"`
	Description    string        `xml:"description"`
	ContentEncoded cdataString   `xml:"content:encoded"`
	ExcerptEncoded cdataString   `xml:"excerpt:encoded,omitempty"`
	PostID         int           `xml:"wp:post_id"`
	PostDate       string        `xml:"wp:post_date"`
	PostDateGMT    string        `xml:"wp:post_date_gmt"`
	CommentStatus  string        `xml:"wp:comment_status"`
	PingStatus     string        `xml:"wp:ping_status"`
	PostName       string        `xml:"wp:post_name"`
	Status         string        `xml:"wp:status"`
	PostParent     int           `xml:"wp:post_parent"`
	MenuOrder      int           `xml:"wp:menu_order"`
	PostType       string        `xml:"wp:post_type"`
	IsSticky       int           `xml:"wp:is_sticky"`
	Categories     []wxrCategory `xml:"category,omitempty"`
	Comments       []wxrComment  `xml:"wp:comment,omitempty"`
}

type wxrCategory struct {
	Domain   string      `xml:"domain,attr"`
	Nicename string      `xml:"nicename,attr"`
	Name     cdataString `xml:",cdata"`
}

type wxrComment struct {
	CommentID          int         `xml:"wp:comment_id"`
	CommentAuthor      cdataString `xml:"wp:comment_author"`
	CommentAuthorEmail string      `xml:"wp:comment_author_email"`
	CommentAuthorURL   string      `xml:"wp:comment_author_url"`
	CommentAuthorIP    string      `xml:"wp:comment_author_IP"`
	CommentDate        string      `xml:"wp:comment_date"`
	CommentDateGMT     string      `xml:"wp:comment_date_gmt"`
	CommentContent     cdataString `xml:"wp:comment_content"`
	CommentApproved    string      `xml:"wp:comment_approved"`
	CommentType        string      `xml:"wp:comment_type"`
	CommentParent      int         `xml:"wp:comment_parent"`
}

type wxrImport struct {
	Channel wxrImportChannel `xml:"channel"`
}

type wxrImportChannel struct {
	BaseSiteURL string          `xml:"http://wordpress.org/export/1.2/ base_site_url"`
	BaseBlogURL string          `xml:"http://wordpress.org/export/1.2/ base_blog_url"`
	Items       []wxrImportItem `xml:"item"`
}

type wxrImportItem struct {
	Title          string              `xml:"title"`
	Link           string              `xml:"link"`
	PubDate        string              `xml:"pubDate"`
	Creator        string              `xml:"http://purl.org/dc/elements/1.1/ creator"`
	GUID           string              `xml:"guid"`
	Description    string              `xml:"description"`
	ContentEncoded string              `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	ExcerptEncoded string              `xml:"http://wordpress.org/export/1.2/excerpt/ encoded"`
	PostID         string              `xml:"http://wordpress.org/export/1.2/ post_id"`
	PostDate       string              `xml:"http://wordpress.org/export/1.2/ post_date"`
	PostDateGMT    string              `xml:"http://wordpress.org/export/1.2/ post_date_gmt"`
	PostName       string              `xml:"http://wordpress.org/export/1.2/ post_name"`
	Status         string              `xml:"http://wordpress.org/export/1.2/ status"`
	PostType       string              `xml:"http://wordpress.org/export/1.2/ post_type"`
	Categories     []wxrImportCategory `xml:"category"`
	Comments       []wxrImportComment  `xml:"http://wordpress.org/export/1.2/ comment"`
}

type wxrImportCategory struct {
	Domain   string `xml:"domain,attr"`
	Nicename string `xml:"nicename,attr"`
	Name     string `xml:",chardata"`
}

type wxrImportComment struct {
	CommentID          string `xml:"http://wordpress.org/export/1.2/ comment_id"`
	CommentAuthor      string `xml:"http://wordpress.org/export/1.2/ comment_author"`
	CommentAuthorEmail string `xml:"http://wordpress.org/export/1.2/ comment_author_email"`
	CommentAuthorURL   string `xml:"http://wordpress.org/export/1.2/ comment_author_url"`
	CommentAuthorIP    string `xml:"http://wordpress.org/export/1.2/ comment_author_IP"`
	CommentDate        string `xml:"http://wordpress.org/export/1.2/ comment_date"`
	CommentDateGMT     string `xml:"http://wordpress.org/export/1.2/ comment_date_gmt"`
	CommentContent     string `xml:"http://wordpress.org/export/1.2/ comment_content"`
	CommentApproved    string `xml:"http://wordpress.org/export/1.2/ comment_approved"`
	CommentType        string `xml:"http://wordpress.org/export/1.2/ comment_type"`
	CommentParent      string `xml:"http://wordpress.org/export/1.2/ comment_parent"`
}

type wxrImportResult struct {
	PostsAdded      int `json:"posts_added"`
	PostsSkipped    int `json:"posts_skipped"`
	CommentsAdded   int `json:"comments_added"`
	CommentsSkipped int `json:"comments_skipped"`
	// Internal tracking (not serialised to JSON).
	importedPostIDs          []string
	postsNeedingDescriptions []string
	postsNeedingTags         []string
	baseSiteURL              string
}

func (s *service) handleAdminExportWXR(w http.ResponseWriter, r *http.Request) {
	posts, err := s.listAllPosts(r.Context())
	if err != nil {
		http.Error(w, "failed to list posts", http.StatusInternalServerError)
		return
	}

	settings := resolveBlogSettings(nil)
	if rawSettings, err := s.store.GetBlogSettings(r.Context()); err == nil {
		settings = resolveBlogSettings(rawSettings)
	}
	commentStatus := "open"
	if !settings.CommentsEnabled {
		commentStatus = "closed"
	}

	baseSiteURL, baseBlogURL := s.resolveBaseURLs(r)
	title := strings.TrimSpace(s.cfg.SiteTitle)
	if title == "" {
		title = "Blog"
	}
	description := strings.TrimSpace(s.cfg.SiteDescription)
	if description == "" {
		description = "Blog export"
	}
	language := strings.TrimSpace(s.cfg.SiteLanguage)
	if language == "" {
		language = "en-US"
	}

	tags := collectTags(posts)

	items := make([]wxrItem, 0, len(posts))
	postID := 1
	commentID := 1
	for _, post := range posts {
		postDate := time.Now().UTC()
		status := "draft"
		if post.PublishedAt != nil {
			postDate = post.PublishedAt.UTC()
			status = "publish"
		}

		contentHTML := strings.TrimSpace(post.ContentHTML)
		if contentHTML == "" && strings.TrimSpace(post.ContentMarkdown) != "" {
			if html, err := markdownToHTML(post.ContentMarkdown); err == nil {
				contentHTML = html
			} else {
				contentHTML = post.ContentMarkdown
			}
		}

		link := strings.TrimSuffix(baseBlogURL, "/") + "/" + strings.TrimPrefix(post.Slug, "/")
		guid := strings.TrimSuffix(baseBlogURL, "/") + "/?p=" + strconv.Itoa(postID)

		categoryNodes := make([]wxrCategory, 0, len(post.Tags))
		for _, tag := range post.Tags {
			slug := strings.TrimSpace(tag.Slug)
			if slug == "" {
				slug = tagSlug(tag.Name)
			}
			categoryNodes = append(categoryNodes, wxrCategory{
				Domain:   "post_tag",
				Nicename: slug,
				Name:     cdataString(tag.Name),
			})
		}

		comments, err := s.store.ListCommentsByPost(r.Context(), post.ID)
		if err != nil {
			http.Error(w, "failed to load comments", http.StatusInternalServerError)
			return
		}

		commentIDMap := map[string]int{}
		for _, c := range comments {
			commentIDMap[c.ID] = commentID
			commentID++
		}

		commentNodes := make([]wxrComment, 0, len(comments))
		for _, c := range comments {
			parentID := 0
			if c.ParentID != nil {
				if mapped, ok := commentIDMap[*c.ParentID]; ok {
					parentID = mapped
				}
			}
			commentNodes = append(commentNodes, wxrComment{
				CommentID:          commentIDMap[c.ID],
				CommentAuthor:      cdataString(c.AuthorName),
				CommentAuthorEmail: "",
				CommentAuthorURL:   "",
				CommentAuthorIP:    "",
				CommentDate:        formatWXRDateTime(c.CreatedAt),
				CommentDateGMT:     formatWXRDateTime(c.CreatedAt.UTC()),
				CommentContent:     cdataString(c.Content),
				CommentApproved:    exportCommentStatus(c.Status),
				CommentType:        "comment",
				CommentParent:      parentID,
			})
		}

		items = append(items, wxrItem{
			Title:          post.Title,
			Link:           link,
			PubDate:        postDate.Format(time.RFC1123Z),
			Creator:        cdataString(defaultExportAuthorLogin(s.cfg.DefaultAuthorLogin)),
			GUID:           wxrGUID{IsPermaLink: "false", Value: guid},
			Description:    "",
			ContentEncoded: cdataString(contentHTML),
			ExcerptEncoded: cdataString(strings.TrimSpace(post.MetaDescription)),
			PostID:         postID,
			PostDate:       formatWXRDateTime(postDate),
			PostDateGMT:    formatWXRDateTime(postDate.UTC()),
			CommentStatus:  commentStatus,
			PingStatus:     "open",
			PostName:       post.Slug,
			Status:         status,
			PostParent:     0,
			MenuOrder:      0,
			PostType:       "post",
			IsSticky:       0,
			Categories:     categoryNodes,
			Comments:       commentNodes,
		})
		postID++
	}

	rss := wxrRSS{
		Version:   "2.0",
		ExcerptNS: wxrExcerptNS,
		ContentNS: wxrContentNS,
		WfwNS:     wxrWfwNS,
		DCNS:      wxrDCNS,
		WPNS:      wxrWPNS,
		Channel: wxrChannel{
			Title:       title,
			Link:        baseBlogURL,
			Description: description,
			PubDate:     time.Now().UTC().Format(time.RFC1123Z),
			Language:    language,
			WXRVersion:  "1.2",
			BaseSiteURL: baseSiteURL,
			BaseBlogURL: baseBlogURL,
			Authors: []wxrAuthor{
				{
					AuthorID:          1,
					AuthorLogin:       defaultExportAuthorLogin(s.cfg.DefaultAuthorLogin),
					AuthorEmail:       "",
					AuthorDisplayName: cdataString(defaultExportAuthorDisplay(s.cfg.DefaultAuthorDisplayName)),
				},
			},
			Tags:  tags,
			Items: items,
		},
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=blog-export.xml")
	_, _ = io.WriteString(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(rss); err != nil {
		http.Error(w, "failed to build export", http.StatusInternalServerError)
		return
	}
}

func (s *service) handleAdminImportWXR(w http.ResponseWriter, r *http.Request) {
	reader, err := readWXRPayload(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		http.Error(w, "failed to read import", http.StatusBadRequest)
		return
	}

	result, err := s.importWXR(r.Context(), payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Queue background task to enrich imported posts.
	if len(result.importedPostIDs) > 0 {
		s.queuePostProcessing("wxr import")
	}
	if result.baseSiteURL != "" && s.cfg.ImageStore != nil && len(result.importedPostIDs) > 0 {
		s.queueImageImport(result.baseSiteURL, result.importedPostIDs)
	}

	writeJSON(w, result)
}

func (s *service) importWXR(ctx context.Context, payload []byte) (wxrImportResult, error) {
	var doc wxrImport
	if err := xml.Unmarshal(payload, &doc); err != nil {
		return wxrImportResult{}, fmt.Errorf("invalid xml: %w", err)
	}

	existingPosts, err := s.listAllPosts(ctx)
	if err != nil {
		return wxrImportResult{}, fmt.Errorf("load posts: %w", err)
	}

	postBySlug := map[string]Post{}
	for _, post := range existingPosts {
		key := normalizeSlugKey(post.Slug)
		if key != "" {
			postBySlug[key] = post
		}
	}

	baseSiteURL := strings.TrimSpace(doc.Channel.BaseBlogURL)
	if baseSiteURL == "" {
		baseSiteURL = strings.TrimSpace(doc.Channel.BaseSiteURL)
	}
	result := wxrImportResult{
		baseSiteURL: baseSiteURL,
	}
	for _, item := range doc.Channel.Items {
		postType := strings.ToLower(strings.TrimSpace(item.PostType))
		if postType == "attachment" {
			continue
		}
		if postType == "" {
			postType = "post"
		}

		slug := importItemSlug(item)
		if slug == "" {
			continue
		}
		slugKey := normalizeSlugKey(slug)
		if slugKey == "" {
			continue
		}

		targetPost, exists := postBySlug[slugKey]
		if exists {
			result.PostsSkipped++
		} else {
			contentHTML := strings.TrimSpace(item.ContentEncoded)
			if contentHTML == "" {
				contentHTML = strings.TrimSpace(item.Description)
			}
			if contentHTML == "" {
				contentHTML = strings.TrimSpace(item.ExcerptEncoded)
			}

			postDate := parseWXRDate(item.PostDateGMT)
			if postDate.IsZero() {
				postDate = parseWXRDate(item.PostDate)
			}
			status := normalizeWXRPostStatus(item.Status)
			var publishedAt *time.Time
			if status == "publish" {
				if postDate.IsZero() {
					now := time.Now().UTC()
					postDate = now
				}
				publishedAt = &postDate
			}

			contentMarkdown := contentHTML
			if md, err := htmlToMarkdown(contentHTML); err == nil && strings.TrimSpace(md) != "" {
				contentMarkdown = md
			}

			post := Post{
				ID:              generateID(),
				Slug:            slug,
				Title:           strings.TrimSpace(item.Title),
				ContentMarkdown: contentMarkdown,
				ContentHTML:     contentHTML,
				PublishedAt:     publishedAt,
				MetaDescription: strings.TrimSpace(firstNonEmpty(item.ExcerptEncoded, item.Description)),
				AuthorID:        defaultImportAuthorID(s.cfg.ImportAuthorID),
			}

			if err := s.store.CreatePost(ctx, &post); err != nil {
				return result, fmt.Errorf("create post: %w", err)
			}
			result.PostsAdded++
			result.importedPostIDs = append(result.importedPostIDs, post.ID)
			if strings.TrimSpace(post.MetaDescription) == "" {
				result.postsNeedingDescriptions = append(result.postsNeedingDescriptions, post.ID)
			}
			postBySlug[slugKey] = post
			targetPost = post

			tagNames := uniqueTagNames(item.Categories)
			if len(tagNames) > 0 {
				if err := s.store.SetPostTags(ctx, post.ID, tagNames); err != nil {
					return result, fmt.Errorf("set tags: %w", err)
				}
			} else if strings.TrimSpace(post.ContentMarkdown) != "" {
				result.postsNeedingTags = append(result.postsNeedingTags, post.ID)
			}
		}

		if targetPost.ID == "" {
			continue
		}

		existingComments, err := s.store.ListCommentsByPost(ctx, targetPost.ID)
		if err != nil {
			return result, fmt.Errorf("load comments: %w", err)
		}
		commentKeys := map[string]bool{}
		for _, c := range existingComments {
			key := commentKey(c.AuthorName, c.Content, c.CreatedAt)
			commentKeys[key] = true
		}

		sortedComments := splitImportComments(item.Comments)
		importedMap := map[string]string{}
		for _, comment := range sortedComments.topLevel {
			createdAt := parseWXRDate(comment.CommentDateGMT)
			if createdAt.IsZero() {
				createdAt = parseWXRDate(comment.CommentDate)
			}
			commentContent := strings.TrimSpace(comment.CommentContent)
			if md, err := htmlToMarkdown(commentContent); err == nil && strings.TrimSpace(md) != "" {
				commentContent = md
			}
			key := commentKey(comment.CommentAuthor, commentContent, createdAt)
			if commentKeys[key] {
				result.CommentsSkipped++
				continue
			}

			newComment := Comment{
				ID:             generateID(),
				PostID:         targetPost.ID,
				ParentID:       nil,
				AuthorName:     strings.TrimSpace(comment.CommentAuthor),
				Content:        commentContent,
				Status:         importCommentStatus(comment.CommentApproved),
				OwnerTokenHash: hashToken(generateToken()),
				CreatedAt:      ensureCommentTime(createdAt),
			}

			if err := s.store.CreateComment(ctx, &newComment); err != nil {
				return result, fmt.Errorf("create comment: %w", err)
			}
			result.CommentsAdded++
			commentKeys[key] = true
			if comment.CommentID != "" {
				importedMap[comment.CommentID] = newComment.ID
			}
		}

		for _, comment := range sortedComments.replies {
			parentID := strings.TrimSpace(comment.CommentParent)
			if parentID == "" || parentID == "0" {
				continue
			}
			mappedParent, ok := importedMap[parentID]
			if !ok {
				continue
			}

			createdAt := parseWXRDate(comment.CommentDateGMT)
			if createdAt.IsZero() {
				createdAt = parseWXRDate(comment.CommentDate)
			}
			commentContent := strings.TrimSpace(comment.CommentContent)
			if md, err := htmlToMarkdown(commentContent); err == nil && strings.TrimSpace(md) != "" {
				commentContent = md
			}
			key := commentKey(comment.CommentAuthor, commentContent, createdAt)
			if commentKeys[key] {
				result.CommentsSkipped++
				continue
			}

			newComment := Comment{
				ID:             generateID(),
				PostID:         targetPost.ID,
				ParentID:       &mappedParent,
				AuthorName:     strings.TrimSpace(comment.CommentAuthor),
				Content:        commentContent,
				Status:         importCommentStatus(comment.CommentApproved),
				OwnerTokenHash: hashToken(generateToken()),
				CreatedAt:      ensureCommentTime(createdAt),
			}

			if err := s.store.CreateComment(ctx, &newComment); err != nil {
				return result, fmt.Errorf("create comment: %w", err)
			}
			result.CommentsAdded++
			commentKeys[key] = true
		}
	}

	return result, nil
}

type splitComments struct {
	topLevel []wxrImportComment
	replies  []wxrImportComment
}

func splitImportComments(comments []wxrImportComment) splitComments {
	out := splitComments{}
	for _, c := range comments {
		commentType := strings.TrimSpace(strings.ToLower(c.CommentType))
		if commentType != "" && commentType != "comment" {
			continue
		}
		if strings.TrimSpace(c.CommentParent) == "" || strings.TrimSpace(c.CommentParent) == "0" {
			out.topLevel = append(out.topLevel, c)
		} else {
			out.replies = append(out.replies, c)
		}
	}
	return out
}

func commentKey(author, content string, createdAt time.Time) string {
	return strings.ToLower(strings.TrimSpace(author)) + "|" + strings.TrimSpace(content) + "|" + createdAt.UTC().Format(time.RFC3339)
}

func ensureCommentTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func parseWXRDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC)
	if err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func formatWXRDateTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func exportCommentStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		return "1"
	case "rejected", "hidden":
		return "0"
	default:
		return "0"
	}
}

func importCommentStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "approved":
		return "approved"
	case "spam", "trash", "rejected":
		return "rejected"
	case "0", "hold", "pending":
		return "pending"
	default:
		return "pending"
	}
}

func normalizeWXRPostStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "publish", "published":
		return "publish"
	case "draft", "pending", "private", "inherit":
		return "draft"
	default:
		return "draft"
	}
}

func collectTags(posts []Post) []wxrTag {
	tags := map[string]Tag{}
	for _, post := range posts {
		for _, tag := range post.Tags {
			slug := strings.TrimSpace(tag.Slug)
			if slug == "" {
				slug = tagSlug(tag.Name)
			}
			if slug == "" {
				continue
			}
			tags[slug] = tag
		}
	}

	out := make([]wxrTag, 0, len(tags))
	idx := 1
	for slug, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			name = slug
		}
		out = append(out, wxrTag{
			TermID: idx,
			Slug:   slug,
			Name:   cdataString(name),
		})
		idx++
	}
	return out
}

func uniqueTagNames(categories []wxrImportCategory) []string {
	seen := map[string]bool{}
	var out []string
	for _, cat := range categories {
		domain := strings.ToLower(strings.TrimSpace(cat.Domain))
		if domain != "category" && domain != "post_tag" && domain != "tag" {
			continue
		}
		name := strings.TrimSpace(cat.Name)
		if name == "" {
			name = strings.TrimSpace(cat.Nicename)
		}
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func normalizeSlugKey(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

func importItemSlug(item wxrImportItem) string {
	slug := strings.TrimSpace(item.PostName)
	if slug == "" {
		slug = extractSlugFromLink(item.Link)
	}
	if slug == "" {
		slug = tagSlug(item.Title)
	}
	return strings.TrimSpace(slug)
}

func extractSlugFromLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if idx := strings.Index(link, "?"); idx >= 0 {
		link = link[:idx]
	}
	link = strings.TrimSuffix(link, "/")
	if idx := strings.LastIndex(link, "/"); idx >= 0 {
		return strings.TrimSpace(link[idx+1:])
	}
	return link
}

func defaultImportAuthorID(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func defaultExportAuthorLogin(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "admin"
	}
	return value
}

func defaultExportAuthorDisplay(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Admin"
	}
	return value
}

func (s *service) resolveBaseURLs(r *http.Request) (string, string) {
	baseSiteURL := strings.TrimSpace(s.cfg.SiteURL)
	if baseSiteURL == "" {
		baseSiteURL = siteURLFromRequest(r)
	}
	baseSiteURL = strings.TrimSuffix(baseSiteURL, "/")

	baseBlogURL := strings.TrimSuffix(baseSiteURL, "/") + s.routePrefix
	return baseSiteURL, baseBlogURL
}

func siteURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		parts := strings.Split(xf, ",")
		scheme = strings.TrimSpace(parts[0])
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func readWXRPayload(r *http.Request) (io.Reader, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, fmt.Errorf("invalid multipart form")
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("missing file")
		}
		return file, nil
	}
	return r.Body, nil
}

func (s *service) listAllPosts(ctx context.Context) ([]Post, error) {
	limit := 200
	offset := 0
	var out []Post
	for {
		posts, err := s.store.ListAllPosts(ctx, limit, offset)
		if err != nil {
			return nil, err
		}
		if len(posts) == 0 {
			break
		}
		out = append(out, posts...)
		offset += len(posts)
	}
	return out, nil
}
