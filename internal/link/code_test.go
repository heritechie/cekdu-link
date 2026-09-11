package link

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGenerateCodeFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := generateCode()
		if err != nil {
			t.Fatalf("generateCode() error = %v", err)
		}
		if code == "" {
			t.Fatal("generated code is empty")
		}
		if len(code) != codeLength {
			t.Errorf("generated code length = %d, want %d", len(code), codeLength)
		}
		for _, c := range code {
			if !strings.ContainsRune(codeAlphabet, c) {
				t.Errorf("generated code contains disallowed character %q", c)
			}
		}
	}
}

func TestCodeAlphabetExcludesAmbiguous(t *testing.T) {
	for _, c := range "0O1Il" {
		if strings.ContainsRune(codeAlphabet, c) {
			t.Errorf("code alphabet contains ambiguous character %q", c)
		}
	}
}

func TestValidateDestinationURL(t *testing.T) {
	valid := []string{
		"http://example.com",
		"https://example.com",
		"https://example.com/path?q=test",
		"http://localhost:3000/x",
		"HTTP://example.com",
	}

	for _, tc := range valid {
		if !validateDestinationURL(tc) {
			t.Errorf("validateDestinationURL(%q) = false, want true", tc)
		}
	}

	invalid := []string{
		"",
		"not-a-url",
		"example.com",
		"javascript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"file:///etc/passwd",
		"ftp://example.com",
		"http://",
		"https:///path",
	}

	for _, tc := range invalid {
		if validateDestinationURL(tc) {
			t.Errorf("validateDestinationURL(%q) = true, want false", tc)
		}
	}
}

func TestValidateCustomCode(t *testing.T) {
	valid := []string{
		"promo-motor",
		"motor_sept",
		"abc123",
		"A_b-c9",
	}

	for _, tc := range valid {
		if err := validateCustomCode(tc); err != nil {
			t.Errorf("validateCustomCode(%q) error = %v, want nil", tc, err)
		}
	}

	invalid := []string{
		"",
		"ab",
		strings.Repeat("a", 65),
		"motor sept",
		"motor/sept",
		"motor?x=1",
		"../admin",
		"login;admin",
		"code%20x",
	}

	for _, tc := range invalid {
		if err := validateCustomCode(tc); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("validateCustomCode(%q) error = %v, want ErrInvalidCode", tc, err)
		}
	}
}

func TestValidateCustomCodeReserved(t *testing.T) {
	for _, tc := range []string{"api", "health", "favicon.ico"} {
		if err := validateCustomCode(tc); !errors.Is(err, ErrReservedCode) {
			t.Errorf("validateCustomCode(%q) error = %v, want ErrReservedCode", tc, err)
		}
	}
}

type fakeRepository struct {
	conflictsBeforeSuccess int
	alwaysConflict         bool
	codes                  []string
	resolveErr             error
	resolveFound           bool
	resolveDestination     string

	listErr     error
	links       []Link
	getErr      error
	getByID     Link
	getCalls    int
	updateErr   error
	updated     Link
	updateCalls int
}

func (f *fakeRepository) Create(_ context.Context, code, destinationURL string) (Link, error) {
	f.codes = append(f.codes, code)
	if f.alwaysConflict {
		return Link{}, ErrCodeConflict
	}
	if f.conflictsBeforeSuccess > 0 {
		f.conflictsBeforeSuccess--
		return Link{}, ErrCodeConflict
	}
	return Link{Code: code, DestinationURL: destinationURL, Status: "active"}, nil
}

func (f *fakeRepository) IncrementActiveClick(_ context.Context, _ string) (string, bool, error) {
	if f.resolveErr != nil {
		return "", false, f.resolveErr
	}
	return f.resolveDestination, f.resolveFound, nil
}

func (f *fakeRepository) List(_ context.Context) ([]Link, error) {
	return f.links, f.listErr
}

func (f *fakeRepository) GetByID(_ context.Context, _ uuid.UUID) (Link, error) {
	f.getCalls++
	if f.getErr != nil {
		return Link{}, f.getErr
	}
	return f.getByID, nil
}

func (f *fakeRepository) Update(_ context.Context, _ uuid.UUID, _ *string, _ *string) (Link, error) {
	f.updateCalls++
	if f.updateErr != nil {
		return Link{}, f.updateErr
	}
	return f.updated, nil
}

func TestServiceCreateRetriesGeneratedCodeOnConflict(t *testing.T) {
	repo := &fakeRepository{conflictsBeforeSuccess: 2}
	svc := NewService(repo)

	l, err := svc.Create(context.Background(), "https://example.com", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(repo.codes) != 3 {
		t.Errorf("insert attempts = %d, want 3", len(repo.codes))
	}
	if repo.codes[0] == repo.codes[1] || repo.codes[1] == repo.codes[2] {
		t.Error("retry reused the same generated code")
	}
	if l.Code != repo.codes[2] {
		t.Errorf("returned code = %q, want %q", l.Code, repo.codes[2])
	}
	if l.DestinationURL != "https://example.com" {
		t.Errorf("destination_url = %q", l.DestinationURL)
	}
	if l.Status != "active" {
		t.Errorf("status = %q, want active", l.Status)
	}
}

func TestServiceCreateExhaustsRetries(t *testing.T) {
	repo := &fakeRepository{alwaysConflict: true}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), "https://example.com", "")
	if !errors.Is(err, ErrCodeConflict) {
		t.Fatalf("Create() error = %v, want ErrCodeConflict", err)
	}
	if len(repo.codes) != maxCodeGenerationAttempts {
		t.Errorf("insert attempts = %d, want %d", len(repo.codes), maxCodeGenerationAttempts)
	}
}

func TestServiceCreateCustomCodeConflictNotRetried(t *testing.T) {
	repo := &fakeRepository{alwaysConflict: true}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), "https://example.com", "promo-motor")
	if !errors.Is(err, ErrCodeConflict) {
		t.Fatalf("Create() error = %v, want ErrCodeConflict", err)
	}
	if len(repo.codes) != 1 {
		t.Errorf("insert attempts = %d, want 1", len(repo.codes))
	}
}

func TestServiceCreateRejectsInvalidDestination(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	for _, tc := range []string{"", "not-a-url", "javascript:alert(1)"} {
		_, err := svc.Create(context.Background(), tc, "")
		if !errors.Is(err, ErrInvalidDestinationURL) {
			t.Errorf("Create(%q) error = %v, want ErrInvalidDestinationURL", tc, err)
		}
	}
	if len(repo.codes) != 0 {
		t.Errorf("repository called %d times, want 0", len(repo.codes))
	}
}

func TestServiceCreateRejectsInvalidCustomCode(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	for _, tc := range []string{"ab", "motor sept", "motor/sept"} {
		_, err := svc.Create(context.Background(), "https://example.com", tc)
		if !errors.Is(err, ErrInvalidCode) {
			t.Errorf("Create(code=%q) error = %v, want ErrInvalidCode", tc, err)
		}
	}
}
