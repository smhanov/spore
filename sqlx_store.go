package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Default schema helper developers can copy into their migrations.
const SchemaBlogEntities = `
CREATE TABLE IF NOT EXISTS blog_entities (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    slug TEXT NULL,
    status TEXT NULL,
    owner_id TEXT NULL,
    parent_id TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL,
    published_at TIMESTAMP NULL,
    attributes JSON NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind ON blog_entities(kind);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_slug ON blog_entities(kind, slug);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_status ON blog_entities(kind, status);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_owner ON blog_entities(kind, owner_id);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_parent ON blog_entities(kind, parent_id);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_created ON blog_entities(kind, created_at);
CREATE INDEX IF NOT EXISTS idx_blog_entities_kind_published ON blog_entities(kind, published_at);
`

// SQLXStore is a reference implementation of BlogStore using sqlx.
type SQLXStore struct {
	DB       *sqlx.DB
	Dialect  string
	keyGuard *regexp.Regexp
}

// NewSQLXStore constructs a store backed by the provided sqlx.DB.
func NewSQLXStore(db *sqlx.DB) *SQLXStore {
	return &SQLXStore{
		DB:       db,
		Dialect:  detectDialect(db),
		keyGuard: regexp.MustCompile(`^[a-zA-Z0-9_]+$`),
	}
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

// Save creates or updates an entity by ID.
func (s *SQLXStore) Save(ctx context.Context, e *Entity) error {
	if e == nil {
		return fmt.Errorf("entity required")
	}
	if strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("entity kind required")
	}
	if e.ID == "" {
		e.ID = generateID()
	}
	if e.Attrs == nil {
		e.Attrs = Attributes{}
	}

	createdAt := e.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := time.Now().UTC()
	if e.UpdatedAt != nil {
		updatedAt = e.UpdatedAt.UTC()
	}

	query := `
INSERT INTO blog_entities (id, kind, slug, status, owner_id, parent_id, created_at, updated_at, published_at, attributes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	kind = excluded.kind,
	slug = excluded.slug,
	status = excluded.status,
	owner_id = excluded.owner_id,
	parent_id = excluded.parent_id,
	updated_at = excluded.updated_at,
	published_at = excluded.published_at,
	attributes = excluded.attributes
`
	query = s.DB.Rebind(query)

	_, err := s.DB.ExecContext(ctx, query,
		e.ID,
		e.Kind,
		nullIfEmpty(e.Slug),
		nullIfEmpty(e.Status),
		nullIfEmpty(e.OwnerID),
		nullIfEmpty(e.ParentID),
		createdAt,
		updatedAt,
		e.PublishedAt,
		e.Attrs,
	)
	return err
}

// Get retrieves a single entity by ID.
func (s *SQLXStore) Get(ctx context.Context, id string) (*Entity, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	var entity Entity
	query := `SELECT id, kind, COALESCE(slug,'') AS slug, COALESCE(status,'') AS status, COALESCE(owner_id,'') AS owner_id, COALESCE(parent_id,'') AS parent_id, created_at, updated_at, published_at, attributes FROM blog_entities WHERE id = ?`
	query = s.DB.Rebind(query)
	if err := s.DB.GetContext(ctx, &entity, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &entity, nil
}

// Find retrieves entities matching a query.
func (s *SQLXStore) Find(ctx context.Context, q Query) ([]*Entity, error) {
	baseQuery := `SELECT id, kind, COALESCE(slug,'') AS slug, COALESCE(status,'') AS status, COALESCE(owner_id,'') AS owner_id, COALESCE(parent_id,'') AS parent_id, created_at, updated_at, published_at, attributes FROM blog_entities`
	var conditions []string
	var args []interface{}

	if strings.TrimSpace(q.Kind) != "" {
		conditions = append(conditions, "kind = ?")
		args = append(args, q.Kind)
	}

	promotedCols := map[string]bool{
		"id":           true,
		"kind":         true,
		"slug":         true,
		"status":       true,
		"owner_id":     true,
		"parent_id":    true,
		"created_at":   true,
		"updated_at":   true,
		"published_at": true,
	}

	for key, val := range q.Filter {
		if promotedCols[key] {
			if val == nil {
				conditions = append(conditions, fmt.Sprintf("%s IS NULL", key))
				continue
			}
			conditions = append(conditions, fmt.Sprintf("%s = ?", key))
			args = append(args, val)
			continue
		}
		if !s.validKey(key) {
			return nil, fmt.Errorf("invalid filter key: %s", key)
		}
		expr := s.jsonExtractExpr(key)
		if val == nil {
			conditions = append(conditions, fmt.Sprintf("%s IS NULL", expr))
			continue
		}
		conditions = append(conditions, fmt.Sprintf("%s = ?", expr))
		args = append(args, val)
	}

	fullQuery := baseQuery
	if len(conditions) > 0 {
		fullQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	if orderBy := sanitizeOrderBy(q.OrderBy); orderBy != "" {
		fullQuery += " ORDER BY " + orderBy
	} else {
		fullQuery += " ORDER BY created_at DESC"
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	fullQuery += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	fullQuery = s.DB.Rebind(fullQuery)

	var entities []*Entity
	if err := s.DB.SelectContext(ctx, &entities, fullQuery, args...); err != nil {
		return nil, err
	}
	return entities, nil
}

// Delete removes an entity by ID.
func (s *SQLXStore) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	query := `DELETE FROM blog_entities WHERE id = ?`
	query = s.DB.Rebind(query)
	_, err := s.DB.ExecContext(ctx, query, id)
	return err
}

func (s *SQLXStore) validKey(key string) bool {
	if s == nil || s.keyGuard == nil {
		return false
	}
	return s.keyGuard.MatchString(key)
}

func (s *SQLXStore) jsonExtractExpr(key string) string {
	if strings.HasPrefix(strings.ToLower(s.Dialect), "postgres") || strings.HasPrefix(strings.ToLower(s.Dialect), "pgx") {
		return fmt.Sprintf("attributes ->> '%s'", key)
	}
	return fmt.Sprintf("json_extract(attributes, '$.%s')", key)
}

func detectDialect(db *sqlx.DB) string {
	if db == nil {
		return "sqlite"
	}
	type driverNamer interface {
		DriverName() string
	}
	if namer, ok := interface{}(db).(driverNamer); ok {
		return namer.DriverName()
	}
	return "sqlite"
}

func sanitizeOrderBy(order string) string {
	fields := strings.Fields(strings.TrimSpace(order))
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 2 {
		return ""
	}
	field := strings.ToLower(fields[0])
	allowed := map[string]bool{
		"id":           true,
		"kind":         true,
		"slug":         true,
		"status":       true,
		"owner_id":     true,
		"parent_id":    true,
		"created_at":   true,
		"updated_at":   true,
		"published_at": true,
	}
	if !allowed[field] {
		return ""
	}
	direction := "ASC"
	if len(fields) == 2 {
		switch strings.ToUpper(fields[1]) {
		case "ASC", "DESC":
			direction = strings.ToUpper(fields[1])
		default:
			return ""
		}
	}
	return field + " " + direction
}

func nullIfEmpty(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
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
