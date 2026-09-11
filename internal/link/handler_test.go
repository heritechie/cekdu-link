package link

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const (
	testDBName   = "cekdu_link_test"
	testBaseURL  = "http://localhost:18080"
	testHostPort = "15432"
)

type testDeps struct {
	pool    *pgxpool.Pool
	handler *Handler
	repo    *Repository
}

func setupTestDB(t *testing.T) *testDeps {
	t.Helper()

	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_TEST_PORT")
	if port == "" {
		port = testHostPort
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "cekdu"
	}
	pass := os.Getenv("DB_PASSWORD")
	if pass == "" {
		pass = "cekdu_local_dev"
	}

	ctx := context.Background()

	adminDSN := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable", user, pass, host, port)
	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres not available, skipping integration test: %v", err)
	}
	defer adminPool.Close()

	if err := adminPool.Ping(ctx); err != nil {
		t.Skipf("postgres not reachable, skipping integration test: %v", err)
	}

	var exists bool
	if err := adminPool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", testDBName,
	).Scan(&exists); err != nil {
		t.Fatalf("check test database: %v", err)
	}
	if !exists {
		if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+testDBName); err != nil {
			t.Fatalf("create test database: %v", err)
		}
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, testDBName)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	goose.SetBaseFS(os.DirFS(migrationsDir))
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose SetDialect: %v", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatalf("run migrations on test database: %v", err)
	}

	if _, err := pool.Exec(ctx, "TRUNCATE links"); err != nil {
		t.Fatalf("truncate links: %v", err)
	}

	repo := NewRepository(pool)
	service := NewService(repo)
	handler := NewHandler(service, testBaseURL)

	return &testDeps{pool: pool, handler: handler, repo: repo}
}

func postJSON(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	muxFor(h).ServeHTTP(rr, req)
	return rr
}

func decodeResponse(t *testing.T, rr *httptest.ResponseRecorder) linkResponse {
	t.Helper()
	var resp linkResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
	return resp
}

func linkRow(t *testing.T, pool *pgxpool.Pool, code string) (destinationURL string, clickCount int64, status string) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		"SELECT destination_url, click_count, status FROM links WHERE code = $1", code,
	).Scan(&destinationURL, &clickCount, &status)
	if err != nil {
		t.Fatalf("query link row %q: %v", code, err)
	}
	return destinationURL, clickCount, status
}

// muxFor builds the application router exactly as cmd/server/main.go does, so
// tests exercise the real path/method precedence and PathValue resolution.
func muxFor(h *Handler) http.Handler {
	health := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("POST /api/links", h.Create)
	mux.HandleFunc("GET /api/links", h.List)
	mux.HandleFunc("GET /api/links/{id}", h.Get)
	mux.HandleFunc("PATCH /api/links/{id}", h.Patch)
	mux.HandleFunc("/{code}", h.Redirect)
	return mux
}

func TestCreateLinkIntegration(t *testing.T) {
	deps := setupTestDB(t)

	t.Run("created with generated code", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com/some-page"}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body=%s)", rr.Code, rr.Body.String())
		}
		if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}

		resp := decodeResponse(t, rr)
		if resp.ID == "" {
			t.Error("id is empty")
		}
		if len(resp.Code) != codeLength {
			t.Errorf("code length = %d, want %d", len(resp.Code), codeLength)
		}
		for _, c := range resp.Code {
			if !strings.ContainsRune(codeAlphabet, c) {
				t.Errorf("code contains disallowed character %q", c)
			}
		}
		if resp.ShortURL != testBaseURL+"/"+resp.Code {
			t.Errorf("short_url = %q, want %q", resp.ShortURL, testBaseURL+"/"+resp.Code)
		}
		if resp.DestinationURL != "https://example.com/some-page" {
			t.Errorf("destination_url = %q", resp.DestinationURL)
		}
		if resp.Status != "active" {
			t.Errorf("status = %q, want active", resp.Status)
		}
		if resp.ClickCount != 0 {
			t.Errorf("click_count = %d, want 0", resp.ClickCount)
		}
		if resp.CreatedAt.IsZero() {
			t.Error("created_at is zero")
		}
		if resp.UpdatedAt.IsZero() {
			t.Error("updated_at is zero")
		}

		dest, clicks, status := linkRow(t, deps.pool, resp.Code)
		if dest != resp.DestinationURL || clicks != 0 || status != "active" {
			t.Errorf("persisted row mismatch: dest=%q clicks=%d status=%q", dest, clicks, status)
		}
	})

	t.Run("created with custom code", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com","code":"promo-motor"}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body=%s)", rr.Code, rr.Body.String())
		}

		resp := decodeResponse(t, rr)
		if resp.Code != "promo-motor" {
			t.Errorf("code = %q, want promo-motor", resp.Code)
		}
		if resp.ShortURL != testBaseURL+"/promo-motor" {
			t.Errorf("short_url = %q", resp.ShortURL)
		}
		dest, clicks, status := linkRow(t, deps.pool, "promo-motor")
		if dest != "https://example.com" || clicks != 0 || status != "active" {
			t.Errorf("persisted row mismatch: dest=%q clicks=%d status=%q", dest, clicks, status)
		}
	})

	t.Run("missing destination_url", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("invalid destination_url", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"not-a-url"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		var er errorResponse
		json.Unmarshal(rr.Body.Bytes(), &er)
		if er.Error != "invalid destination_url" {
			t.Errorf("error = %q", er.Error)
		}
	})

	t.Run("unsupported url scheme", func(t *testing.T) {
		for _, scheme := range []string{"javascript:alert(1)", "data:text/html,hi", "file:///etc/passwd", "ftp://example.com"} {
			rr := postJSON(t, deps.handler, fmt.Sprintf(`{"destination_url":%q}`, scheme))
			if rr.Code != http.StatusBadRequest {
				t.Errorf("destination %q: status = %d, want 400", scheme, rr.Code)
			}
		}
	})

	t.Run("invalid custom code", func(t *testing.T) {
		for _, code := range []string{"motor sept", "motor/sept", "motor?x=1", "../admin"} {
			rr := postJSON(t, deps.handler, fmt.Sprintf(`{"destination_url":"https://example.com","code":%q}`, code))
			if rr.Code != http.StatusBadRequest {
				t.Errorf("code %q: status = %d, want 400", code, rr.Code)
			}
		}
	})

	t.Run("custom code too short", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com","code":"ab"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("custom code too long", func(t *testing.T) {
		long := strings.Repeat("a", 65)
		rr := postJSON(t, deps.handler, fmt.Sprintf(`{"destination_url":"https://example.com","code":%q}`, long))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("reserved custom code", func(t *testing.T) {
		for _, code := range []string{"api", "health", "favicon.ico"} {
			rr := postJSON(t, deps.handler, fmt.Sprintf(`{"destination_url":"https://example.com","code":%q}`, code))
			if rr.Code != http.StatusBadRequest {
				t.Errorf("code %q: status = %d, want 400", code, rr.Code)
			}
		}
	})

	t.Run("duplicate custom code", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com","code":"dup-code"}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("first create status = %d, want 201 (body=%s)", rr.Code, rr.Body.String())
		}

		rr = postJSON(t, deps.handler, `{"destination_url":"https://other.example.com","code":"dup-code"}`)
		if rr.Code != http.StatusConflict {
			t.Fatalf("second create status = %d, want 409", rr.Code)
		}
		var er errorResponse
		json.Unmarshal(rr.Body.Bytes(), &er)
		if er.Error != "code already exists" {
			t.Errorf("error = %q, want code already exists", er.Error)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com","extra":123}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("trailing json content", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com"} {"destination_url":"https://other.com"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		rr := postJSON(t, deps.handler, ``)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("unsupported method", func(t *testing.T) {
		for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
			req := httptest.NewRequest(method, "/api/links", strings.NewReader(`{"destination_url":"https://example.com"}`))
			rr := httptest.NewRecorder()
			muxFor(deps.handler).ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("method %s: status = %d, want 405", method, rr.Code)
			}
			if allow := rr.Header().Get("Allow"); !strings.Contains(allow, http.MethodPost) {
				t.Errorf("method %s: Allow = %q, want it to include POST", method, allow)
			}
		}
	})
}

func TestShortURLConstruction(t *testing.T) {
	deps := setupTestDB(t)
	service := NewService(deps.repo)

	for i, base := range []string{"https://cekdu.lu/", "https://cekdu.lu///", "https://cekdu.lu"} {
		h := NewHandler(service, base)

		code := fmt.Sprintf("slashtest%d", i)
		req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(fmt.Sprintf(`{"destination_url":"https://example.com","code":%q}`, code)))
		rr := httptest.NewRecorder()
		muxFor(h).ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("base %q: status = %d (body=%s)", base, rr.Code, rr.Body.String())
		}

		var resp struct {
			ShortURL string `json:"short_url"`
		}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		want := "https://cekdu.lu/" + code
		if resp.ShortURL != want {
			t.Errorf("base %q: short_url = %q, want %q", base, resp.ShortURL, want)
		}
	}
}

func TestRepositoryCreateUniqueConflict(t *testing.T) {
	deps := setupTestDB(t)

	if _, err := deps.repo.Create(context.Background(), "repo-conflict", "https://example.com"); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, err := deps.repo.Create(context.Background(), "repo-conflict", "https://other.com"); err != ErrCodeConflict {
		t.Fatalf("second Create() error = %v, want ErrCodeConflict", err)
	}
}

func TestRepositoryCreatePersistsDefaults(t *testing.T) {
	deps := setupTestDB(t)

	l, err := deps.repo.Create(context.Background(), "repo-defaults", "https://example.com")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if l.ID.String() == "" {
		t.Error("id is empty")
	}
	if l.ClickCount != 0 {
		t.Errorf("click_count = %d, want 0", l.ClickCount)
	}
	if l.Status != "active" {
		t.Errorf("status = %q, want active", l.Status)
	}
	if l.CreatedAt.IsZero() || l.UpdatedAt.IsZero() {
		t.Error("timestamps are zero")
	}

	var rowCount int
	err = deps.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM links WHERE code = $1", "repo-defaults",
	).Scan(&rowCount)
	if err != nil {
		t.Fatalf("count links: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("persisted rows = %d, want 1", rowCount)
	}
}
