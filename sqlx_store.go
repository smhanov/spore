package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Default schema helpers developers can copy into their migrations.
const (
	SchemaBlogPosts = `
CREATE TABLE IF NOT EXISTS blog_posts (
    id TEXT PRIMARY KEY,
    slug TEXT UNIQUE NOT NULL,
    title TEXT NOT NULL,
    content_markdown TEXT NOT NULL,
    content_html TEXT NOT NULL,
    published_at TIMESTAMP NULL,
    meta_description TEXT,
    author_id INTEGER NOT NULL
);
`
	SchemaBlogTags = `
CREATE TABLE IF NOT EXISTS blog_tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL
);
`
	SchemaBlogPostTags = `
CREATE TABLE IF NOT EXISTS blog_post_tags (
	post_id TEXT NOT NULL REFERENCES blog_posts(id) ON DELETE CASCADE,
	tag_id TEXT NOT NULL REFERENCES blog_tags(id) ON DELETE CASCADE,
	PRIMARY KEY (post_id, tag_id)
);
`
	SchemaBlogAISettings = `
CREATE TABLE IF NOT EXISTS blog_ai_settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	smart_provider TEXT,
	smart_model TEXT,
	smart_api_key TEXT,
	smart_base_url TEXT,
	smart_temperature REAL,
	smart_max_tokens INTEGER,
	dumb_provider TEXT,
	dumb_model TEXT,
	dumb_api_key TEXT,
	dumb_base_url TEXT,
	dumb_temperature REAL,
	dumb_max_tokens INTEGER,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	SchemaBlogSettings = `
CREATE TABLE IF NOT EXISTS blog_settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	comments_enabled BOOLEAN NOT NULL DEFAULT TRUE,
	date_display TEXT NOT NULL DEFAULT 'absolute',
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	SchemaBlogComments = `
CREATE TABLE IF NOT EXISTS blog_comments (
	id TEXT PRIMARY KEY,
	post_id TEXT NOT NULL REFERENCES blog_posts(id) ON DELETE CASCADE,
	parent_id TEXT NULL REFERENCES blog_comments(id) ON DELETE CASCADE,
	author_name TEXT NOT NULL,
	content TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'approved',
	owner_token_hash TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NULL,
	spam_checked_at TIMESTAMP NULL,
	spam_reason TEXT NULL
);
CREATE INDEX IF NOT EXISTS idx_blog_comments_post_id ON blog_comments(post_id);
CREATE INDEX IF NOT EXISTS idx_blog_comments_status ON blog_comments(status);
CREATE INDEX IF NOT EXISTS idx_blog_comments_parent_id ON blog_comments(parent_id);
`
)

// SQLXStore is a reference implementation of BlogStore using sqlx.
type SQLXStore struct {
	DB *sqlx.DB
}

// NewSQLXStore constructs a store backed by the provided sqlx.DB.
func NewSQLXStore(db *sqlx.DB) *SQLXStore {
	return &SQLXStore{DB: db}
}

// Migrate applies the built-in migrations for the SQLX store.
func (s *SQLXStore) Migrate(ctx context.Context) (err error) {
	if s == nil || s.DB == nil {
		return fmt.Errorf("sqlx store requires a database")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS blog_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	rows, err := tx.QueryxContext(ctx, `SELECT version FROM blog_migrations`)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if scanErr := rows.Scan(&version); scanErr != nil {
			return fmt.Errorf("scan migration version: %w", scanErr)
		}
		applied[version] = true
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return fmt.Errorf("read migrations: %w", rowsErr)
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		for _, stmt := range m.Statements {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err = tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO blog_migrations (version, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, m.Version, m.Name); err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func (s *SQLXStore) GetPublishedPostBySlug(ctx context.Context, slug string) (*Post, error) {
	var p Post
	err := s.DB.GetContext(ctx, &p, `SELECT id, slug, title, content_markdown, content_html, published_at, meta_description, author_id FROM blog_posts WHERE slug=$1 AND published_at IS NOT NULL`, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	tags, err := s.GetPostTags(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Tags = tags
	return &p, nil
}

func (s *SQLXStore) ListPublishedPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	posts := []Post{}
	err := s.DB.SelectContext(ctx, &posts, `SELECT id, slug, title, content_markdown, content_html, published_at, meta_description, author_id FROM blog_posts WHERE published_at IS NOT NULL ORDER BY published_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	if err := s.LoadPostsTags(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (s *SQLXStore) ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error) {
	posts := []Post{}
	err := s.DB.SelectContext(ctx, &posts, `
SELECT p.id, p.slug, p.title, p.content_markdown, p.content_html, p.published_at, p.meta_description, p.author_id
FROM blog_posts p
JOIN blog_post_tags pt ON pt.post_id = p.id
JOIN blog_tags t ON t.id = pt.tag_id
WHERE t.slug = $1 AND p.published_at IS NOT NULL
ORDER BY p.published_at DESC
LIMIT $2 OFFSET $3`, tagSlug, limit, offset)
	if err != nil {
		return nil, err
	}
	if err := s.LoadPostsTags(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (s *SQLXStore) CreatePost(ctx context.Context, p *Post) error {
	if p.ID == "" {
		p.ID = generateID()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO blog_posts (id, slug, title, content_markdown, content_html, published_at, meta_description, author_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.Slug, p.Title, p.ContentMarkdown, p.ContentHTML, p.PublishedAt, p.MetaDescription, p.AuthorID)
	return err
}

func (s *SQLXStore) UpdatePost(ctx context.Context, p *Post) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE blog_posts SET slug=$1, title=$2, content_markdown=$3, content_html=$4, published_at=$5, meta_description=$6, author_id=$7 WHERE id=$8`,
		p.Slug, p.Title, p.ContentMarkdown, p.ContentHTML, p.PublishedAt, p.MetaDescription, p.AuthorID, p.ID)
	return err
}

func (s *SQLXStore) GetPostByID(ctx context.Context, id string) (*Post, error) {
	var p Post
	err := s.DB.GetContext(ctx, &p, `SELECT id, slug, title, content_markdown, content_html, published_at, meta_description, author_id FROM blog_posts WHERE id=$1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	tags, err := s.GetPostTags(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Tags = tags
	return &p, nil
}

func (s *SQLXStore) DeletePost(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM blog_posts WHERE id=$1`, id)
	return err
}

func (s *SQLXStore) ListAllPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	posts := []Post{}
	err := s.DB.SelectContext(ctx, &posts, `SELECT id, slug, title, content_markdown, content_html, published_at, meta_description, author_id FROM blog_posts ORDER BY COALESCE(published_at, '9999-12-31') DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	if err := s.LoadPostsTags(ctx, posts); err != nil {
		return nil, err
	}
	return posts, nil
}

type aiSettingsRow struct {
	ID               int      `db:"id"`
	SmartProvider    string   `db:"smart_provider"`
	SmartModel       string   `db:"smart_model"`
	SmartAPIKey      string   `db:"smart_api_key"`
	SmartBaseURL     string   `db:"smart_base_url"`
	SmartTemperature *float64 `db:"smart_temperature"`
	SmartMaxTokens   *int     `db:"smart_max_tokens"`
	DumbProvider     string   `db:"dumb_provider"`
	DumbModel        string   `db:"dumb_model"`
	DumbAPIKey       string   `db:"dumb_api_key"`
	DumbBaseURL      string   `db:"dumb_base_url"`
	DumbTemperature  *float64 `db:"dumb_temperature"`
	DumbMaxTokens    *int     `db:"dumb_max_tokens"`
}

func (s *SQLXStore) GetAISettings(ctx context.Context) (*AISettings, error) {
	var row aiSettingsRow
	err := s.DB.GetContext(ctx, &row, `SELECT id, smart_provider, smart_model, smart_api_key, smart_base_url, smart_temperature, smart_max_tokens, dumb_provider, dumb_model, dumb_api_key, dumb_base_url, dumb_temperature, dumb_max_tokens FROM blog_ai_settings WHERE id = 1`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	settings := &AISettings{
		Smart: AIProviderSettings{
			Provider:    row.SmartProvider,
			Model:       row.SmartModel,
			APIKey:      row.SmartAPIKey,
			BaseURL:     row.SmartBaseURL,
			Temperature: row.SmartTemperature,
			MaxTokens:   row.SmartMaxTokens,
		},
		Dumb: AIProviderSettings{
			Provider:    row.DumbProvider,
			Model:       row.DumbModel,
			APIKey:      row.DumbAPIKey,
			BaseURL:     row.DumbBaseURL,
			Temperature: row.DumbTemperature,
			MaxTokens:   row.DumbMaxTokens,
		},
	}
	return settings, nil
}

func (s *SQLXStore) UpdateAISettings(ctx context.Context, settings *AISettings) error {
	if settings == nil {
		return fmt.Errorf("ai settings required")
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO blog_ai_settings (
    id, smart_provider, smart_model, smart_api_key, smart_base_url, smart_temperature, smart_max_tokens,
    dumb_provider, dumb_model, dumb_api_key, dumb_base_url, dumb_temperature, dumb_max_tokens
) VALUES (
    1, $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12
) ON CONFLICT(id) DO UPDATE SET
    smart_provider = excluded.smart_provider,
    smart_model = excluded.smart_model,
    smart_api_key = excluded.smart_api_key,
    smart_base_url = excluded.smart_base_url,
    smart_temperature = excluded.smart_temperature,
    smart_max_tokens = excluded.smart_max_tokens,
    dumb_provider = excluded.dumb_provider,
    dumb_model = excluded.dumb_model,
    dumb_api_key = excluded.dumb_api_key,
    dumb_base_url = excluded.dumb_base_url,
    dumb_temperature = excluded.dumb_temperature,
    dumb_max_tokens = excluded.dumb_max_tokens,
    updated_at = CURRENT_TIMESTAMP
`,
		settings.Smart.Provider,
		settings.Smart.Model,
		settings.Smart.APIKey,
		settings.Smart.BaseURL,
		settings.Smart.Temperature,
		settings.Smart.MaxTokens,
		settings.Dumb.Provider,
		settings.Dumb.Model,
		settings.Dumb.APIKey,
		settings.Dumb.BaseURL,
		settings.Dumb.Temperature,
		settings.Dumb.MaxTokens,
	)
	return err
}

func (s *SQLXStore) GetBlogSettings(ctx context.Context) (*BlogSettings, error) {
	var settings BlogSettings
	err := s.DB.GetContext(ctx, &settings, `SELECT comments_enabled, date_display FROM blog_settings WHERE id = 1`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if settings.DateDisplay == "" {
		settings.DateDisplay = "absolute"
	}
	return &settings, nil
}

func (s *SQLXStore) UpdateBlogSettings(ctx context.Context, settings *BlogSettings) error {
	if settings == nil {
		return fmt.Errorf("blog settings required")
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO blog_settings (id, comments_enabled, date_display)
VALUES (1, $1, $2)
ON CONFLICT(id) DO UPDATE SET
    comments_enabled = excluded.comments_enabled,
    date_display = excluded.date_display,
    updated_at = CURRENT_TIMESTAMP
`, settings.CommentsEnabled, settings.DateDisplay)
	return err
}

func (s *SQLXStore) CreateComment(ctx context.Context, c *Comment) error {
	if c == nil {
		return fmt.Errorf("comment required")
	}
	if c.ID == "" {
		c.ID = generateID()
	}
	if c.Status == "" {
		c.Status = "approved"
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO blog_comments (
    id, post_id, parent_id, author_name, content, status, owner_token_hash
) VALUES ($1,$2,$3,$4,$5,$6,$7)
`, c.ID, c.PostID, c.ParentID, c.AuthorName, c.Content, c.Status, c.OwnerTokenHash)
	return err
}

func (s *SQLXStore) GetCommentByID(ctx context.Context, id string) (*Comment, error) {
	var c Comment
	err := s.DB.GetContext(ctx, &c, `
SELECT id, post_id, parent_id, author_name, content, status, owner_token_hash, created_at, updated_at, spam_checked_at, spam_reason
FROM blog_comments WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *SQLXStore) ListCommentsByPost(ctx context.Context, postID string) ([]Comment, error) {
	comments := []Comment{}
	err := s.DB.SelectContext(ctx, &comments, `
SELECT id, post_id, parent_id, author_name, content, status, owner_token_hash, created_at, updated_at, spam_checked_at, spam_reason
FROM blog_comments WHERE post_id = $1
ORDER BY created_at ASC`, postID)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (s *SQLXStore) UpdateCommentContentByOwner(ctx context.Context, id, ownerTokenHash, content string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `
UPDATE blog_comments
SET content = $1, updated_at = CURRENT_TIMESTAMP
WHERE id = $2 AND owner_token_hash = $3 AND status != 'rejected'
`, content, id, ownerTokenHash)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *SQLXStore) DeleteCommentByOwner(ctx context.Context, id, ownerTokenHash string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM blog_comments WHERE id = $1 AND owner_token_hash = $2`, id, ownerTokenHash)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *SQLXStore) UpdateCommentStatus(ctx context.Context, id, status string, spamReason *string) error {
	_, err := s.DB.ExecContext(ctx, `
UPDATE blog_comments
SET status = $1,
    spam_reason = $2,
    updated_at = CURRENT_TIMESTAMP,
    spam_checked_at = CASE WHEN $1 IN ('approved', 'rejected') THEN CURRENT_TIMESTAMP ELSE spam_checked_at END
WHERE id = $3
`, status, spamReason, id)
	return err
}

func (s *SQLXStore) ListCommentsForModeration(ctx context.Context, status string, limit, offset int) ([]AdminComment, error) {
	comments := []AdminComment{}
	query := `
SELECT c.id, c.post_id, c.parent_id, c.author_name, c.content, c.status, c.owner_token_hash, c.created_at, c.updated_at, c.spam_checked_at, c.spam_reason,
       p.title AS post_title, p.slug AS post_slug
FROM blog_comments c
JOIN blog_posts p ON p.id = c.post_id`
	args := []any{}
	if strings.TrimSpace(status) != "" {
		query += " WHERE c.status = $1"
		args = append(args, status)
	}
	query += " ORDER BY c.created_at DESC LIMIT $" + fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)
	args = append(args, limit, offset)

	err := s.DB.SelectContext(ctx, &comments, query, args...)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (s *SQLXStore) DeleteCommentByID(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM blog_comments WHERE id = $1`, id)
	return err
}

// SetPostTags replaces all tags for a post. Creates new tags as needed.
func (s *SQLXStore) SetPostTags(ctx context.Context, postID string, tagNames []string) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Remove existing tags for the post
	if _, err = tx.ExecContext(ctx, `DELETE FROM blog_post_tags WHERE post_id = $1`, postID); err != nil {
		return err
	}

	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		slug := tagSlug(name)

		// Upsert the tag
		var tagID string
		err = tx.GetContext(ctx, &tagID, `SELECT id FROM blog_tags WHERE slug = $1`, slug)
		if err != nil {
			tagID = generateID()
			if _, err = tx.ExecContext(ctx, `INSERT INTO blog_tags (id, name, slug) VALUES ($1, $2, $3)`, tagID, name, slug); err != nil {
				return err
			}
		}

		// Link tag to post
		if _, err = tx.ExecContext(ctx, `INSERT INTO blog_post_tags (post_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, postID, tagID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetPostTags returns all tags for a given post.
func (s *SQLXStore) GetPostTags(ctx context.Context, postID string) ([]Tag, error) {
	tags := []Tag{}
	err := s.DB.SelectContext(ctx, &tags, `
SELECT t.id, t.name, t.slug
FROM blog_tags t
JOIN blog_post_tags pt ON pt.tag_id = t.id
WHERE pt.post_id = $1
ORDER BY t.name`, postID)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// LoadPostsTags populates the Tags field on each post in the slice.
func (s *SQLXStore) LoadPostsTags(ctx context.Context, posts []Post) error {
	if len(posts) == 0 {
		return nil
	}
	ids := make([]string, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}

	type postTag struct {
		PostID string `db:"post_id"`
		Tag
	}

	query, args, err := sqlx.In(`
SELECT pt.post_id, t.id, t.name, t.slug
FROM blog_tags t
JOIN blog_post_tags pt ON pt.tag_id = t.id
WHERE pt.post_id IN (?)
ORDER BY t.name`, ids)
	if err != nil {
		return err
	}
	query = s.DB.Rebind(query)

	var rows []postTag
	if err := s.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return err
	}

	tagMap := map[string][]Tag{}
	for _, r := range rows {
		tagMap[r.PostID] = append(tagMap[r.PostID], r.Tag)
	}
	for i := range posts {
		posts[i].Tags = tagMap[posts[i].ID]
		if posts[i].Tags == nil {
			posts[i].Tags = []Tag{}
		}
	}
	return nil
}

// GetRelatedPosts finds posts related to the given post based on shared tags.
func (s *SQLXStore) GetRelatedPosts(ctx context.Context, postID string, limit int) ([]Post, error) {
	posts := []Post{}
	err := s.DB.SelectContext(ctx, &posts, `
SELECT p.id, p.slug, p.title, p.content_markdown, p.content_html, p.published_at, p.meta_description, p.author_id
FROM blog_posts p
JOIN blog_post_tags pt ON pt.post_id = p.id
JOIN blog_post_tags pt2 ON pt2.tag_id = pt.tag_id AND pt2.post_id = $1
WHERE p.id != $1 AND p.published_at IS NOT NULL
GROUP BY p.id
ORDER BY COUNT(pt.tag_id) DESC, p.published_at DESC
LIMIT $2`, postID, limit)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// tagSlug converts a tag name to a URL-friendly slug.
func tagSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
