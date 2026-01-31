package blog

import (
	"context"
	"database/sql"
	"errors"

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
)

// SQLXStore is a reference implementation of BlogStore using sqlx.
type SQLXStore struct {
	DB *sqlx.DB
}

// NewSQLXStore constructs a store backed by the provided sqlx.DB.
func NewSQLXStore(db *sqlx.DB) *SQLXStore {
	return &SQLXStore{DB: db}
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
