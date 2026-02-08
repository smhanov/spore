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
	return &p, nil
}

func (s *SQLXStore) ListPublishedPosts(ctx context.Context, limit, offset int) ([]Post, error) {
	posts := []Post{}
	err := s.DB.SelectContext(ctx, &posts, `SELECT id, slug, title, content_markdown, content_html, published_at, meta_description, author_id FROM blog_posts WHERE published_at IS NOT NULL ORDER BY published_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
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
