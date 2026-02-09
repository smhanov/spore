package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	entityKindPost    = "post"
	entityKindComment = "comment"
	entityKindTask    = "task"
	entityKindSetting = "setting"

	entityIDAISettings   = "settings-ai"
	entityIDBlogSettings = "settings-blog"
)

type storeAdapter struct {
	store BlogStore
}

func newStoreAdapter(store BlogStore) *storeAdapter {
	return &storeAdapter{store: store}
}

type postAttrs struct {
	Title           string `json:"title"`
	ContentMarkdown string `json:"content_markdown"`
	ContentHTML     string `json:"content_html"`
	MetaDescription string `json:"meta_description"`
	AuthorID        int    `json:"author_id"`
	Tags            []Tag  `json:"tags"`
}

type commentAttrs struct {
	AuthorName     string     `json:"author_name"`
	Content        string     `json:"content"`
	OwnerTokenHash string     `json:"owner_token_hash"`
	SpamCheckedAt  *time.Time `json:"spam_checked_at,omitempty"`
	SpamReason     *string    `json:"spam_reason,omitempty"`
}

type taskAttrs struct {
	TaskType     string  `json:"task_type"`
	Payload      string  `json:"payload"`
	Result       string  `json:"result"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

type aiSettingsAttrs struct {
	Smart AIProviderSettings `json:"smart"`
	Dumb  AIProviderSettings `json:"dumb"`
}

type blogSettingsAttrs struct {
	CommentsEnabled bool   `json:"comments_enabled"`
	DateDisplay     string `json:"date_display"`
}

func decodeAttrs(attrs Attributes, target interface{}) error {
	if attrs == nil {
		return nil
	}
	payload, err := json.Marshal(attrs)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func postStatus(p *Post) string {
	if p != nil && p.PublishedAt != nil {
		return "published"
	}
	return "draft"
}

func entityFromPost(p *Post) *Entity {
	if p == nil {
		return nil
	}
	now := time.Now().UTC()
	p.UpdatedAt = &now
	attrs := postAttrs{
		Title:           p.Title,
		ContentMarkdown: p.ContentMarkdown,
		ContentHTML:     p.ContentHTML,
		MetaDescription: p.MetaDescription,
		AuthorID:        p.AuthorID,
		Tags:            p.Tags,
	}
	return &Entity{
		ID:          p.ID,
		Kind:        entityKindPost,
		Slug:        p.Slug,
		Status:      postStatus(p),
		PublishedAt: p.PublishedAt,
		UpdatedAt:   p.UpdatedAt,
		Attrs: Attributes{
			"title":            attrs.Title,
			"content_markdown": attrs.ContentMarkdown,
			"content_html":     attrs.ContentHTML,
			"meta_description": attrs.MetaDescription,
			"author_id":        attrs.AuthorID,
			"tags":             attrs.Tags,
		},
	}
}

func entityToPost(e *Entity) (*Post, error) {
	if e == nil {
		return nil, nil
	}
	var attrs postAttrs
	if err := decodeAttrs(e.Attrs, &attrs); err != nil {
		return nil, err
	}
	if attrs.Tags == nil {
		attrs.Tags = []Tag{}
	}
	return &Post{
		ID:              e.ID,
		Slug:            e.Slug,
		Title:           attrs.Title,
		ContentMarkdown: attrs.ContentMarkdown,
		ContentHTML:     attrs.ContentHTML,
		PublishedAt:     e.PublishedAt,
		UpdatedAt:       e.UpdatedAt,
		MetaDescription: attrs.MetaDescription,
		AuthorID:        attrs.AuthorID,
		Tags:            attrs.Tags,
	}, nil
}

func entityFromComment(c *Comment) *Entity {
	if c == nil {
		return nil
	}
	attrs := commentAttrs{
		AuthorName:     c.AuthorName,
		Content:        c.Content,
		OwnerTokenHash: c.OwnerTokenHash,
		SpamCheckedAt:  c.SpamCheckedAt,
		SpamReason:     c.SpamReason,
	}
	return &Entity{
		ID:        c.ID,
		Kind:      entityKindComment,
		Status:    c.Status,
		OwnerID:   c.PostID,
		ParentID:  valueOrEmpty(c.ParentID),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Attrs: Attributes{
			"author_name":      attrs.AuthorName,
			"content":          attrs.Content,
			"owner_token_hash": attrs.OwnerTokenHash,
			"spam_checked_at":  attrs.SpamCheckedAt,
			"spam_reason":      attrs.SpamReason,
		},
	}
}

func entityToComment(e *Entity) (*Comment, error) {
	if e == nil {
		return nil, nil
	}
	var attrs commentAttrs
	if err := decodeAttrs(e.Attrs, &attrs); err != nil {
		return nil, err
	}
	comment := &Comment{
		ID:             e.ID,
		PostID:         e.OwnerID,
		AuthorName:     attrs.AuthorName,
		Content:        attrs.Content,
		Status:         e.Status,
		OwnerTokenHash: attrs.OwnerTokenHash,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		SpamCheckedAt:  attrs.SpamCheckedAt,
		SpamReason:     attrs.SpamReason,
	}
	if strings.TrimSpace(e.ParentID) != "" {
		parent := e.ParentID
		comment.ParentID = &parent
	}
	return comment, nil
}

func entityFromTask(t *Task) *Entity {
	if t == nil {
		return nil
	}
	attrs := taskAttrs{
		TaskType:     t.TaskType,
		Payload:      t.Payload,
		Result:       t.Result,
		ErrorMessage: t.ErrorMessage,
	}
	return &Entity{
		ID:        t.ID,
		Kind:      entityKindTask,
		Status:    t.Status,
		CreatedAt: t.CreatedAt,
		UpdatedAt: &t.UpdatedAt,
		Attrs: Attributes{
			"task_type":     attrs.TaskType,
			"payload":       attrs.Payload,
			"result":        attrs.Result,
			"error_message": attrs.ErrorMessage,
		},
	}
}

func entityToTask(e *Entity) (*Task, error) {
	if e == nil {
		return nil, nil
	}
	var attrs taskAttrs
	if err := decodeAttrs(e.Attrs, &attrs); err != nil {
		return nil, err
	}
	task := &Task{
		ID:           e.ID,
		TaskType:     attrs.TaskType,
		Status:       e.Status,
		Payload:      attrs.Payload,
		Result:       attrs.Result,
		ErrorMessage: attrs.ErrorMessage,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    resolvedTime(e.UpdatedAt, e.CreatedAt),
	}
	return task, nil
}

func entityFromAISettings(settings *AISettings) *Entity {
	attrs := aiSettingsAttrs{}
	if settings != nil {
		attrs.Smart = settings.Smart
		attrs.Dumb = settings.Dumb
	}
	return &Entity{
		ID:   entityIDAISettings,
		Kind: entityKindSetting,
		Attrs: Attributes{
			"smart": attrs.Smart,
			"dumb":  attrs.Dumb,
		},
	}
}

func entityToAISettings(e *Entity) (*AISettings, error) {
	if e == nil {
		return nil, nil
	}
	var attrs aiSettingsAttrs
	if err := decodeAttrs(e.Attrs, &attrs); err != nil {
		return nil, err
	}
	return &AISettings{Smart: attrs.Smart, Dumb: attrs.Dumb}, nil
}

func entityFromBlogSettings(settings *BlogSettings) *Entity {
	attrs := blogSettingsAttrs{}
	if settings != nil {
		attrs.CommentsEnabled = settings.CommentsEnabled
		attrs.DateDisplay = settings.DateDisplay
	}
	return &Entity{
		ID:   entityIDBlogSettings,
		Kind: entityKindSetting,
		Attrs: Attributes{
			"comments_enabled": attrs.CommentsEnabled,
			"date_display":     attrs.DateDisplay,
		},
	}
}

func entityToBlogSettings(e *Entity) (*BlogSettings, error) {
	if e == nil {
		return nil, nil
	}
	var attrs blogSettingsAttrs
	if err := decodeAttrs(e.Attrs, &attrs); err != nil {
		return nil, err
	}
	return &BlogSettings{CommentsEnabled: attrs.CommentsEnabled, DateDisplay: attrs.DateDisplay}, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func resolvedTime(value *time.Time, fallback time.Time) time.Time {
	if value != nil {
		return *value
	}
	return fallback
}

func (a *storeAdapter) GetPublishedPostBySlug(ctx context.Context, slug string) (*Post, error) {
	q := Query{
		Kind: entityKindPost,
		Filter: map[string]interface{}{
			"slug":   slug,
			"status": "published",
		},
		Limit: 1,
	}
	entities, err := a.store.Find(ctx, q)
	if err != nil || len(entities) == 0 {
		return nil, err
	}
	return entityToPost(entities[0])
}

func (a *storeAdapter) ListPublishedPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	q := Query{
		Kind: entityKindPost,
		Filter: map[string]interface{}{
			"status": "published",
		},
		Limit:   limit,
		Offset:  offset,
		OrderBy: "published_at DESC",
	}
	entities, err := a.store.Find(ctx, q)
	if err != nil {
		return nil, err
	}
	return entitiesToPosts(entities)
}

func (a *storeAdapter) ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error) {
	filterFn := func(post Post) bool {
		for _, tag := range post.Tags {
			if strings.EqualFold(tag.Slug, tagSlug) {
				return true
			}
		}
		return false
	}
	return a.collectPublishedPosts(ctx, limit, offset, filterFn)
}

func (a *storeAdapter) CreatePost(ctx context.Context, p *Post) error {
	if p == nil {
		return fmt.Errorf("post required")
	}
	if p.ID == "" {
		p.ID = generateID()
	}
	entity := entityFromPost(p)
	if entity == nil {
		return fmt.Errorf("post entity required")
	}
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) UpdatePost(ctx context.Context, p *Post) error {
	if p == nil {
		return fmt.Errorf("post required")
	}
	entity := entityFromPost(p)
	if entity == nil {
		return fmt.Errorf("post entity required")
	}
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) GetPostByID(ctx context.Context, id string) (*Post, error) {
	entity, err := a.store.Get(ctx, id)
	if err != nil || entity == nil {
		return nil, err
	}
	if entity.Kind != entityKindPost {
		return nil, nil
	}
	return entityToPost(entity)
}

func (a *storeAdapter) DeletePost(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

func (a *storeAdapter) ListAllPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	entities, err := a.fetchAllEntities(ctx, entityKindPost)
	if err != nil {
		return nil, err
	}
	posts, err := entitiesToPosts(entities)
	if err != nil {
		return nil, err
	}
	posts = sortPostsForAdmin(posts)
	return slicePosts(posts, limit, offset), nil
}

func (a *storeAdapter) SetPostTags(ctx context.Context, postID string, tagNames []string) error {
	post, err := a.GetPostByID(ctx, postID)
	if err != nil || post == nil {
		return err
	}
	var tags []Tag
	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		slug := tagSlug(name)
		if slug == "" {
			continue
		}
		tags = append(tags, Tag{ID: slug, Name: name, Slug: slug})
	}
	post.Tags = tags
	return a.UpdatePost(ctx, post)
}

func (a *storeAdapter) GetPostTags(ctx context.Context, postID string) ([]Tag, error) {
	post, err := a.GetPostByID(ctx, postID)
	if err != nil || post == nil {
		return []Tag{}, err
	}
	if post.Tags == nil {
		return []Tag{}, nil
	}
	return post.Tags, nil
}

func (a *storeAdapter) LoadPostsTags(ctx context.Context, posts []Post) error {
	for i := range posts {
		if posts[i].Tags == nil {
			posts[i].Tags = []Tag{}
		}
	}
	return nil
}

func (a *storeAdapter) GetRelatedPosts(ctx context.Context, postID string, limit int) ([]Post, error) {
	post, err := a.GetPostByID(ctx, postID)
	if err != nil || post == nil {
		return nil, err
	}
	if len(post.Tags) == 0 {
		return []Post{}, nil
	}

	entities, err := a.fetchAllEntities(ctx, entityKindPost)
	if err != nil {
		return nil, err
	}
	posts, err := entitiesToPosts(entities)
	if err != nil {
		return nil, err
	}

	targetTags := tagSlugSet(post.Tags)
	type scored struct {
		post  Post
		score int
	}
	var scoredPosts []scored
	for _, candidate := range posts {
		if candidate.ID == postID || candidate.PublishedAt == nil {
			continue
		}
		score := countSharedTags(targetTags, candidate.Tags)
		if score == 0 {
			continue
		}
		scoredPosts = append(scoredPosts, scored{post: candidate, score: score})
	}

	sort.Slice(scoredPosts, func(i, j int) bool {
		if scoredPosts[i].score != scoredPosts[j].score {
			return scoredPosts[i].score > scoredPosts[j].score
		}
		return publishedAtOrZero(scoredPosts[i].post).After(publishedAtOrZero(scoredPosts[j].post))
	})

	if limit <= 0 || limit > len(scoredPosts) {
		limit = len(scoredPosts)
	}
	out := make([]Post, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scoredPosts[i].post)
	}
	return out, nil
}

func (a *storeAdapter) GetAISettings(ctx context.Context) (*AISettings, error) {
	entity, err := a.store.Get(ctx, entityIDAISettings)
	if err != nil || entity == nil {
		return nil, err
	}
	return entityToAISettings(entity)
}

func (a *storeAdapter) UpdateAISettings(ctx context.Context, settings *AISettings) error {
	entity := entityFromAISettings(settings)
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) GetBlogSettings(ctx context.Context) (*BlogSettings, error) {
	entity, err := a.store.Get(ctx, entityIDBlogSettings)
	if err != nil || entity == nil {
		return nil, err
	}
	return entityToBlogSettings(entity)
}

func (a *storeAdapter) UpdateBlogSettings(ctx context.Context, settings *BlogSettings) error {
	entity := entityFromBlogSettings(settings)
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) CreateComment(ctx context.Context, c *Comment) error {
	if c == nil {
		return fmt.Errorf("comment required")
	}
	if c.ID == "" {
		c.ID = generateID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(c.Status) == "" {
		c.Status = "approved"
	}
	entity := entityFromComment(c)
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) GetCommentByID(ctx context.Context, id string) (*Comment, error) {
	entity, err := a.store.Get(ctx, id)
	if err != nil || entity == nil {
		return nil, err
	}
	if entity.Kind != entityKindComment {
		return nil, nil
	}
	return entityToComment(entity)
}

func (a *storeAdapter) ListCommentsByPost(ctx context.Context, postID string) ([]Comment, error) {
	var all []*Entity
	offset := 0
	for {
		q := Query{
			Kind: entityKindComment,
			Filter: map[string]interface{}{
				"owner_id": postID,
			},
			Limit:   200,
			Offset:  offset,
			OrderBy: "created_at ASC",
		}
		entities, err := a.store.Find(ctx, q)
		if err != nil {
			return nil, err
		}
		if len(entities) == 0 {
			break
		}
		all = append(all, entities...)
		offset += len(entities)
	}
	return entitiesToComments(all)
}

func (a *storeAdapter) UpdateCommentContentByOwner(ctx context.Context, id, ownerTokenHash, content string) (bool, error) {
	comment, err := a.GetCommentByID(ctx, id)
	if err != nil || comment == nil {
		return false, err
	}
	if comment.OwnerTokenHash != ownerTokenHash {
		return false, nil
	}
	now := time.Now().UTC()
	comment.Content = content
	comment.UpdatedAt = &now
	entity := entityFromComment(comment)
	return true, a.store.Save(ctx, entity)
}

func (a *storeAdapter) DeleteCommentByOwner(ctx context.Context, id, ownerTokenHash string) (bool, error) {
	comment, err := a.GetCommentByID(ctx, id)
	if err != nil || comment == nil {
		return false, err
	}
	if comment.OwnerTokenHash != ownerTokenHash {
		return false, nil
	}
	return true, a.store.Delete(ctx, id)
}

func (a *storeAdapter) UpdateCommentStatus(ctx context.Context, id, status string, spamReason *string) error {
	comment, err := a.GetCommentByID(ctx, id)
	if err != nil || comment == nil {
		return err
	}
	now := time.Now().UTC()
	comment.Status = status
	comment.SpamReason = spamReason
	comment.SpamCheckedAt = &now
	comment.UpdatedAt = &now
	entity := entityFromComment(comment)
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) ListCommentsForModeration(ctx context.Context, status string, limit, offset int) ([]AdminComment, error) {
	filter := map[string]interface{}{}
	if strings.TrimSpace(status) != "" {
		filter["status"] = status
	}
	q := Query{
		Kind:    entityKindComment,
		Filter:  filter,
		Limit:   limit,
		Offset:  offset,
		OrderBy: "created_at DESC",
	}
	entities, err := a.store.Find(ctx, q)
	if err != nil {
		return nil, err
	}
	comments, err := entitiesToComments(entities)
	if err != nil {
		return nil, err
	}

	postCache := map[string]*Post{}
	out := make([]AdminComment, 0, len(comments))
	for _, comment := range comments {
		postID := comment.PostID
		post := postCache[postID]
		if post == nil {
			loaded, err := a.GetPostByID(ctx, postID)
			if err != nil {
				return nil, err
			}
			post = loaded
			postCache[postID] = post
		}
		admin := AdminComment{Comment: comment}
		if post != nil {
			admin.PostTitle = post.Title
			admin.PostSlug = post.Slug
		}
		out = append(out, admin)
	}
	return out, nil
}

func (a *storeAdapter) DeleteCommentByID(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

func (a *storeAdapter) CreateTask(ctx context.Context, task *Task) error {
	if task == nil {
		return fmt.Errorf("task required")
	}
	if task.ID == "" {
		task.ID = generateID()
	}
	if strings.TrimSpace(task.Status) == "" {
		task.Status = TaskStatusPending
	}
	if task.Payload == "" {
		task.Payload = "{}"
	}
	if task.Result == "" {
		task.Result = "{}"
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	entity := entityFromTask(task)
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) GetTask(ctx context.Context, id string) (*Task, error) {
	entity, err := a.store.Get(ctx, id)
	if err != nil || entity == nil {
		return nil, err
	}
	if entity.Kind != entityKindTask {
		return nil, nil
	}
	return entityToTask(entity)
}

func (a *storeAdapter) ListPendingTasks(ctx context.Context) ([]Task, error) {
	q := Query{
		Kind: entityKindTask,
		Filter: map[string]interface{}{
			"status": TaskStatusPending,
		},
		Limit:   50,
		OrderBy: "created_at ASC",
	}
	entities, err := a.store.Find(ctx, q)
	if err != nil {
		return nil, err
	}
	return entitiesToTasks(entities)
}

func (a *storeAdapter) ListRecentTasks(ctx context.Context, limit int) ([]Task, error) {
	q := Query{
		Kind:    entityKindTask,
		Limit:   limit,
		OrderBy: "created_at DESC",
	}
	entities, err := a.store.Find(ctx, q)
	if err != nil {
		return nil, err
	}
	return entitiesToTasks(entities)
}

func (a *storeAdapter) UpdateTask(ctx context.Context, task *Task) error {
	if task == nil {
		return fmt.Errorf("task required")
	}
	if task.ID == "" {
		return fmt.Errorf("task id required")
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = time.Now().UTC()
	}
	entity := entityFromTask(task)
	return a.store.Save(ctx, entity)
}

func (a *storeAdapter) ResetRunningTasks(ctx context.Context) error {
	q := Query{
		Kind: entityKindTask,
		Filter: map[string]interface{}{
			"status": TaskStatusRunning,
		},
		Limit: 200,
	}
	entities, err := a.store.Find(ctx, q)
	if err != nil {
		return err
	}
	for _, entity := range entities {
		task, err := entityToTask(entity)
		if err != nil {
			return err
		}
		task.Status = TaskStatusPending
		task.UpdatedAt = time.Now().UTC()
		if err := a.UpdateTask(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

func (a *storeAdapter) fetchAllEntities(ctx context.Context, kind string) ([]*Entity, error) {
	var out []*Entity
	offset := 0
	for {
		q := Query{Kind: kind, Limit: 200, Offset: offset, OrderBy: "created_at DESC"}
		entities, err := a.store.Find(ctx, q)
		if err != nil {
			return nil, err
		}
		if len(entities) == 0 {
			break
		}
		out = append(out, entities...)
		offset += len(entities)
	}
	return out, nil
}

func entitiesToPosts(entities []*Entity) ([]Post, error) {
	posts := make([]Post, 0, len(entities))
	for _, entity := range entities {
		post, err := entityToPost(entity)
		if err != nil {
			return nil, err
		}
		if post != nil {
			posts = append(posts, *post)
		}
	}
	return posts, nil
}

func entitiesToComments(entities []*Entity) ([]Comment, error) {
	comments := make([]Comment, 0, len(entities))
	for _, entity := range entities {
		comment, err := entityToComment(entity)
		if err != nil {
			return nil, err
		}
		if comment != nil {
			comments = append(comments, *comment)
		}
	}
	return comments, nil
}

func entitiesToTasks(entities []*Entity) ([]Task, error) {
	tasks := make([]Task, 0, len(entities))
	for _, entity := range entities {
		task, err := entityToTask(entity)
		if err != nil {
			return nil, err
		}
		if task != nil {
			tasks = append(tasks, *task)
		}
	}
	return tasks, nil
}

func (a *storeAdapter) collectPublishedPosts(ctx context.Context, limit, offset int, filterFn func(Post) bool) ([]Post, error) {
	var out []Post
	totalOffset := offset
	page := 0
	for {
		q := Query{
			Kind: entityKindPost,
			Filter: map[string]interface{}{
				"status": "published",
			},
			Limit:   100,
			Offset:  page * 100,
			OrderBy: "published_at DESC",
		}
		entities, err := a.store.Find(ctx, q)
		if err != nil {
			return nil, err
		}
		if len(entities) == 0 {
			break
		}
		posts, err := entitiesToPosts(entities)
		if err != nil {
			return nil, err
		}
		for _, post := range posts {
			if !filterFn(post) {
				continue
			}
			if totalOffset > 0 {
				totalOffset--
				continue
			}
			out = append(out, post)
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
		page++
	}
	return out, nil
}

func sortPostsForAdmin(posts []Post) []Post {
	sort.Slice(posts, func(i, j int) bool {
		left := adminSortTime(posts[i])
		right := adminSortTime(posts[j])
		if left.Equal(right) {
			return posts[i].ID < posts[j].ID
		}
		return left.After(right)
	})
	return posts
}

func adminSortTime(post Post) time.Time {
	if post.PublishedAt != nil {
		return post.PublishedAt.UTC()
	}
	return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
}

func slicePosts(posts []Post, limit, offset int) []Post {
	if offset >= len(posts) {
		return []Post{}
	}
	if offset < 0 {
		offset = 0
	}
	end := len(posts)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return posts[offset:end]
}

func tagSlugSet(tags []Tag) map[string]bool {
	set := map[string]bool{}
	for _, tag := range tags {
		slug := strings.TrimSpace(tag.Slug)
		if slug == "" {
			slug = tagSlug(tag.Name)
		}
		if slug != "" {
			set[strings.ToLower(slug)] = true
		}
	}
	return set
}

func countSharedTags(target map[string]bool, tags []Tag) int {
	count := 0
	for _, tag := range tags {
		slug := strings.TrimSpace(tag.Slug)
		if slug == "" {
			slug = tagSlug(tag.Name)
		}
		if slug == "" {
			continue
		}
		if target[strings.ToLower(slug)] {
			count++
		}
	}
	return count
}

func publishedAtOrZero(post Post) time.Time {
	if post.PublishedAt != nil {
		return post.PublishedAt.UTC()
	}
	return time.Time{}
}
