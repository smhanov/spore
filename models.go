package blog

import "time"

// Post represents a blog post with both markdown source and pre-rendered HTML for fast serving.
type Post struct {
	ID              string     `json:"id" db:"id"`
	Slug            string     `json:"slug" db:"slug"`
	Title           string     `json:"title" db:"title"`
	ContentMarkdown string     `json:"content_markdown" db:"content_markdown"`
	ContentHTML     string     `json:"content_html" db:"content_html"`
	PublishedAt     *time.Time `json:"published_at" db:"published_at"`
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
