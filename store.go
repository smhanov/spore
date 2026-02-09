package blog

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// ImageStore defines an optional interface for storing and retrieving images.
// If not provided, image upload functionality will be disabled.
type ImageStore interface {
	// SaveImage stores an image and returns its URL/path for retrieval.
	// The id is a unique identifier for the image (e.g., UUID).
	// The filename is the original filename uploaded by the user.
	// The contentType is the MIME type of the image.
	// The reader provides the image data.
	SaveImage(ctx context.Context, id, filename, contentType string, reader io.Reader) (url string, err error)

	// GetImage retrieves an image by its ID.
	// Returns the content type, reader, and any error.
	GetImage(ctx context.Context, id string) (contentType string, reader io.ReadCloser, err error)

	// DeleteImage removes an image by its ID.
	DeleteImage(ctx context.Context, id string) error
}

// Attributes stores flexible per-entity data as JSON.
type Attributes map[string]interface{}

// Value serializes Attributes for database storage.
func (a Attributes) Value() (driver.Value, error) {
	if a == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(a)
}

// Scan deserializes Attributes from database values.
func (a *Attributes) Scan(value interface{}) error {
	if value == nil {
		*a = Attributes{}
		return nil
	}
	switch raw := value.(type) {
	case []byte:
		if len(raw) == 0 {
			*a = Attributes{}
			return nil
		}
		return json.Unmarshal(raw, a)
	case string:
		if raw == "" {
			*a = Attributes{}
			return nil
		}
		return json.Unmarshal([]byte(raw), a)
	default:
		return errors.New("unsupported attributes type")
	}
}

// Entity represents any object in the system (post, comment, task, settings).
type Entity struct {
	ID          string     `json:"id" db:"id"`
	Kind        string     `json:"kind" db:"kind"`
	Slug        string     `json:"slug,omitempty" db:"slug"`
	Status      string     `json:"status,omitempty" db:"status"`
	OwnerID     string     `json:"owner_id,omitempty" db:"owner_id"`
	ParentID    string     `json:"parent_id,omitempty" db:"parent_id"`
	CreatedAt   time.Time  `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`
	Attrs       Attributes `json:"attrs" db:"attributes"`
}

// Query replaces specific list/get methods with a single flexible filter.
type Query struct {
	Kind    string                 // Filter by entity kind (post, comment, task)
	Filter  map[string]interface{} // Equality filters (promoted columns or attrs)
	Limit   int
	Offset  int
	OrderBy string // e.g., "created_at DESC"
}

// BlogStore defines the minimal persistence contract the host application must satisfy.
type BlogStore interface {
	// Migrate applies any pending migrations required by the store implementation.
	// Implementations should be idempotent and safe to call on every startup.
	Migrate(ctx context.Context) error

	// Save creates or updates an entity. Implementations should upsert by ID.
	Save(ctx context.Context, e *Entity) error

	// Get retrieves a single entity by its ID.
	Get(ctx context.Context, id string) (*Entity, error)

	// Find retrieves entities matching a query.
	Find(ctx context.Context, q Query) ([]*Entity, error)

	// Delete removes an entity by ID.
	Delete(ctx context.Context, id string) error
}
