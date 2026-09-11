package link

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const linkColumns = "id, code, destination_url, click_count, status, created_at, updated_at"

// scanLink scans a single row from a query that selected linkColumns into l.
func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.Code, &l.DestinationURL, &l.ClickCount, &l.Status, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

func (r *Repository) Create(ctx context.Context, code, destinationURL string) (Link, error) {
	l, err := scanLink(r.pool.QueryRow(ctx,
		`INSERT INTO links (code, destination_url)
		 VALUES ($1, $2)
		 RETURNING `+linkColumns,
		code, destinationURL,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "links_code_key" {
			return Link{}, ErrCodeConflict
		}
		return Link{}, fmt.Errorf("insert link: %w", err)
	}
	return l, nil
}

// IncrementActiveClick atomically increments click_count and returns the
// destination URL, but only for existing links with status 'active'.
// It returns found=false for unknown or inactive links without touching them.
func (r *Repository) IncrementActiveClick(ctx context.Context, code string) (destinationURL string, found bool, err error) {
	err = r.pool.QueryRow(ctx,
		`UPDATE links
		 SET click_count = click_count + 1
		 WHERE code = $1 AND status = 'active'
		 RETURNING destination_url`,
		code,
	).Scan(&destinationURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("increment click: %w", err)
	}
	return destinationURL, true, nil
}

// List returns all links ordered by newest first.
func (r *Repository) List(ctx context.Context) ([]Link, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+linkColumns+`
		 FROM links
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	return links, nil
}

// GetByID returns the link with the given id, or ErrLinkNotFound.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Link, error) {
	l, err := scanLink(r.pool.QueryRow(ctx,
		`SELECT `+linkColumns+`
		 FROM links
		 WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrLinkNotFound
	}
	if err != nil {
		return Link{}, fmt.Errorf("get link %s: %w", id, err)
	}
	return l, nil
}

// Update changes destination_url and/or status for the link with the given id.
// Nil pointers leave the corresponding field untouched. updated_at is only
// refreshed when an actual change is made. Returns ErrLinkNotFound for unknown
// ids.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, destinationURL, status *string) (Link, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("begin update link %s: %w", id, err)
	}
	defer tx.Rollback(ctx)

	l, err := scanLink(tx.QueryRow(ctx,
		`SELECT `+linkColumns+`
		 FROM links
		 WHERE id = $1
		 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrLinkNotFound
	}
	if err != nil {
		return Link{}, fmt.Errorf("get link %s: %w", id, err)
	}

	changed := false
	if destinationURL != nil && *destinationURL != l.DestinationURL {
		l.DestinationURL = *destinationURL
		changed = true
	}
	if status != nil && *status != l.Status {
		l.Status = *status
		changed = true
	}

	if !changed {
		return l, nil
	}

	updated, err := scanLink(tx.QueryRow(ctx,
		`UPDATE links
		 SET destination_url = $1, status = $2, updated_at = now()
		 WHERE id = $3
		 RETURNING `+linkColumns,
		l.DestinationURL, l.Status, id))
	if err != nil {
		return Link{}, fmt.Errorf("update link %s: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Link{}, fmt.Errorf("commit update link %s: %w", id, err)
	}
	return updated, nil
}
