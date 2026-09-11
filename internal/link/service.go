package link

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type repository interface {
	Create(ctx context.Context, code, destinationURL string) (Link, error)
	IncrementActiveClick(ctx context.Context, code string) (destinationURL string, found bool, err error)
	List(ctx context.Context) ([]Link, error)
	GetByID(ctx context.Context, id uuid.UUID) (Link, error)
	Update(ctx context.Context, id uuid.UUID, destinationURL, status *string) (Link, error)
}

type Service struct {
	repo repository
}

func NewService(repo repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, destinationURL, customCode string) (Link, error) {
	if !validateDestinationURL(destinationURL) {
		return Link{}, ErrInvalidDestinationURL
	}

	if customCode != "" {
		if err := validateCustomCode(customCode); err != nil {
			return Link{}, err
		}
		return s.repo.Create(ctx, customCode, destinationURL)
	}

	var lastErr error
	for attempt := 0; attempt < maxCodeGenerationAttempts; attempt++ {
		code, err := generateCode()
		if err != nil {
			return Link{}, err
		}

		l, err := s.repo.Create(ctx, code, destinationURL)
		if err == nil {
			return l, nil
		}
		if errors.Is(err, ErrCodeConflict) {
			lastErr = err
			continue
		}
		return Link{}, err
	}

	return Link{}, lastErr
}

// Resolve looks up an active link and atomically records one click for it,
// returning the destination URL. Unknown and inactive links resolve to
// ErrLinkNotFound and are never counted.
func (s *Service) Resolve(ctx context.Context, code string) (string, error) {
	destinationURL, found, err := s.repo.IncrementActiveClick(ctx, code)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrLinkNotFound
	}
	return destinationURL, nil
}

// List returns all links, newest first.
func (s *Service) List(ctx context.Context) ([]Link, error) {
	return s.repo.List(ctx)
}

// Get returns the link with the given id, or ErrLinkNotFound.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Link, error) {
	return s.repo.GetByID(ctx, id)
}

// Update validates and applies a patch to the link with the given id. Nil
// pointers leave the corresponding field untouched.
func (s *Service) Update(ctx context.Context, id uuid.UUID, destinationURL, status *string) (Link, error) {
	if destinationURL != nil && !validateDestinationURL(*destinationURL) {
		return Link{}, ErrInvalidDestinationURL
	}
	if status != nil && !validStatus(*status) {
		return Link{}, ErrInvalidStatus
	}
	return s.repo.Update(ctx, id, destinationURL, status)
}
