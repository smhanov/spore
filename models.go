package blog

import "time"

// Post represents a blog post with both markdown source and pre-rendered HTML for fast serving.
type Post struct {
	ID              string     `json:"id" db:"id"`
	Slug            string     `json:"slug" db:"slug"`
	Title           string     `json:"title" db:"title"`
	Subtitle        string     `json:"subtitle" db:"subtitle"`
	ContentMarkdown string     `json:"content_markdown" db:"content_markdown"`
	ContentHTML     string     `json:"content_html" db:"content_html"`
	PublishedAt     *time.Time `json:"published_at" db:"published_at"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	MetaDescription string     `json:"meta_description" db:"meta_description"`
	AuthorID        int        `json:"author_id" db:"author_id"`
	Tags            []Tag      `json:"tags"`
}

// Tag represents a simple keyword.
type Tag struct {
	ID   string `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
	Slug string `json:"slug" db:"slug"`
}

// AIProviderSettings holds configuration for a single LLM provider.
type AIProviderSettings struct {
	Provider    string   `json:"provider" db:"provider"`
	Model       string   `json:"model" db:"model"`
	APIKey      string   `json:"api_key" db:"api_key"`
	BaseURL     string   `json:"base_url" db:"base_url"`
	Temperature *float64 `json:"temperature" db:"temperature"`
	MaxTokens   *int     `json:"max_tokens" db:"max_tokens"`
}

// AISettings stores the smart and dumb LLM configurations.
type AISettings struct {
	Smart AIProviderSettings `json:"smart"`
	Dumb  AIProviderSettings `json:"dumb"`
}

// BlogSettings stores runtime configuration for the blog.
type BlogSettings struct {
	CommentsEnabled bool   `json:"comments_enabled" db:"comments_enabled"`
	DateDisplay     string `json:"date_display" db:"date_display"`
	Title           string `json:"title" db:"title"`
	Description     string `json:"description" db:"description"`
}

// Comment represents a public comment on a blog post.
type Comment struct {
	ID             string     `json:"id" db:"id"`
	PostID         string     `json:"post_id" db:"post_id"`
	ParentID       *string    `json:"parent_id,omitempty" db:"parent_id"`
	AuthorName     string     `json:"author_name" db:"author_name"`
	Content        string     `json:"content" db:"content"`
	Status         string     `json:"status" db:"status"`
	OwnerTokenHash string     `json:"-" db:"owner_token_hash"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	SpamCheckedAt  *time.Time `json:"spam_checked_at,omitempty" db:"spam_checked_at"`
	SpamReason     *string    `json:"spam_reason,omitempty" db:"spam_reason"`
}

// AdminComment adds post metadata for moderation views.
type AdminComment struct {
	Comment
	PostTitle string `json:"post_title" db:"post_title"`
	PostSlug  string `json:"post_slug" db:"post_slug"`
}

// PostSummary wraps a Post with pre-calculated fields for card/list layouts.
type PostSummary struct {
	Post
	FirstImage string `json:"first_image"`
	Excerpt    string `json:"excerpt"`
}

// Pagination holds page navigation state for list templates.
type Pagination struct {
	CurrentPage int    `json:"current_page"`
	TotalPages  int    `json:"total_pages"`
	NextPageURL string `json:"next_page_url,omitempty"`
	PrevPageURL string `json:"prev_page_url,omitempty"`
}

// Task represents an asynchronous background task that can be persisted and resumed.
type Task struct {
	ID           string    `json:"id" db:"id"`
	TaskType     string    `json:"task_type" db:"task_type"`
	Status       string    `json:"status" db:"status"`
	Payload      string    `json:"payload" db:"payload"`
	Result       string    `json:"result" db:"result"`
	ErrorMessage *string   `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
