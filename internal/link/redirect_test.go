package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func doRequest(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func setInactive(t *testing.T, deps *testDeps, code string) {
	t.Helper()
	_, err := deps.pool.Exec(context.Background(),
		"UPDATE links SET status = 'inactive' WHERE code = $1", code)
	if err != nil {
		t.Fatalf("set link inactive: %v", err)
	}
}

func clickCount(t *testing.T, deps *testDeps, code string) int64 {
	t.Helper()
	var count int64
	err := deps.pool.QueryRow(context.Background(),
		"SELECT click_count FROM links WHERE code = $1", code,
	).Scan(&count)
	if err != nil {
		t.Fatalf("read click_count for %q: %v", code, err)
	}
	return count
}

func TestServiceResolve(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		repo := &fakeRepository{resolveFound: true, resolveDestination: "https://example.com"}
		svc := NewService(repo)
		dest, err := svc.Resolve(context.Background(), "some-code")
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if dest != "https://example.com" {
			t.Errorf("destination = %q", dest)
		}
	})

	t.Run("not found", func(t *testing.T) {
		repo := &fakeRepository{resolveFound: false}
		svc := NewService(repo)
		_, err := svc.Resolve(context.Background(), "missing")
		if !errors.Is(err, ErrLinkNotFound) {
			t.Fatalf("Resolve() error = %v, want ErrLinkNotFound", err)
		}
	})

	t.Run("inactive treated as not found", func(t *testing.T) {
		repo := &fakeRepository{resolveFound: false}
		svc := NewService(repo)
		_, err := svc.Resolve(context.Background(), "inactive")
		if !errors.Is(err, ErrLinkNotFound) {
			t.Fatalf("Resolve() error = %v, want ErrLinkNotFound", err)
		}
	})

	t.Run("database error", func(t *testing.T) {
		repo := &fakeRepository{resolveErr: errors.New("db down")}
		svc := NewService(repo)
		_, err := svc.Resolve(context.Background(), "any")
		if err == nil || !strings.Contains(err.Error(), "db down") {
			t.Fatalf("Resolve() error = %v, want db error", err)
		}
	})
}

func TestRedirectDatabaseFailure(t *testing.T) {
	repo := &fakeRepository{resolveErr: errors.New("postgres unavailable")}
	svc := NewService(repo)
	h := NewHandler(svc, testBaseURL)

	req := httptest.NewRequest(http.MethodGet, "/some-code", nil)
	rr := httptest.NewRecorder()
	h.Redirect(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "" {
		t.Errorf("Location = %q, want empty (must not redirect)", loc)
	}
	var er errorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if er.Error != "internal server error" {
		t.Errorf("error = %q", er.Error)
	}
}

func TestRedirectIntegration(t *testing.T) {
	deps := setupTestDB(t)
	mux := muxFor(deps.handler)

	create := func(code, destination string) {
		t.Helper()
		if _, err := deps.repo.Create(context.Background(), code, destination); err != nil {
			t.Fatalf("create link %q: %v", code, err)
		}
	}

	t.Run("active link redirects with Location", func(t *testing.T) {
		create("redirect-test", "https://example.com")
		rr := doRequest(mux, http.MethodGet, "/redirect-test")
		if rr.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302 (body=%s)", rr.Code, rr.Body.String())
		}
		if loc := rr.Header().Get("Location"); loc != "https://example.com" {
			t.Errorf("Location = %q, want https://example.com", loc)
		}
	})

	t.Run("click count increments exactly once per request", func(t *testing.T) {
		create("click-count", "https://example.com")
		if c := clickCount(t, deps, "click-count"); c != 0 {
			t.Fatalf("initial click_count = %d, want 0", c)
		}
		for want := int64(1); want <= 2; want++ {
			rr := doRequest(mux, http.MethodGet, "/click-count")
			if rr.Code != http.StatusFound {
				t.Fatalf("request %d: status = %d, want 302", want, rr.Code)
			}
			if c := clickCount(t, deps, "click-count"); c != want {
				t.Errorf("after request %d: click_count = %d, want %d", want, c, want)
			}
		}
	})

	t.Run("unknown code returns 404", func(t *testing.T) {
		rr := doRequest(mux, http.MethodGet, "/does-not-exist")
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
		var er errorResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &er); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if er.Error != "link not found" {
			t.Errorf("error = %q, want link not found", er.Error)
		}
	})

	t.Run("inactive link returns 404 and does not count", func(t *testing.T) {
		create("inactive-click", "https://example.com")
		setInactive(t, deps, "inactive-click")
		rr := doRequest(mux, http.MethodGet, "/inactive-click")
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
		if c := clickCount(t, deps, "inactive-click"); c != 0 {
			t.Errorf("inactive link click_count = %d, want 0", c)
		}
	})

	t.Run("unsupported methods return 405", func(t *testing.T) {
		create("methods-test", "https://example.com")
		for _, method := range []string{
			http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch,
		} {
			rr := doRequest(mux, method, "/methods-test")
			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("method %s: status = %d, want 405", method, rr.Code)
			}
			if allow := rr.Header().Get("Allow"); allow != http.MethodGet {
				t.Errorf("method %s: Allow = %q, want GET", method, allow)
			}
		}
		// click_count must not change from rejected methods
		if c := clickCount(t, deps, "methods-test"); c != 0 {
			t.Errorf("click_count after rejected methods = %d, want 0", c)
		}
	})
}

func TestRoutingKeepsSystemRoutes(t *testing.T) {
	deps := setupTestDB(t)
	mux := muxFor(deps.handler)

	if _, err := deps.repo.Create(context.Background(), "route-test", "https://example.com"); err != nil {
		t.Fatalf("create link: %v", err)
	}

	t.Run("health reaches health handler", func(t *testing.T) {
		rr := doRequest(mux, http.MethodGet, "/health")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
	})

	t.Run("GET on api route lists links, not a code lookup", func(t *testing.T) {
		rr := doRequest(mux, http.MethodGet, "/api/links")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (must not be treated as a code)", rr.Code)
		}
		var list []linkResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode list: %v (body=%s)", err, rr.Body.String())
		}
		found := false
		for _, l := range list {
			if l.Code == "route-test" {
				found = true
			}
		}
		if !found {
			t.Errorf("list does not contain route-test: %+v", list)
		}
	})

	t.Run("post to api still creates links", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/links",
			strings.NewReader(`{"destination_url":"https://example.com","code":"api-route-test"}`))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body=%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("code route redirects", func(t *testing.T) {
		rr := doRequest(mux, http.MethodGet, "/route-test")
		if rr.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "https://example.com" {
			t.Errorf("Location = %q", loc)
		}
	})

	t.Run("multi-segment path is not a code", func(t *testing.T) {
		rr := doRequest(mux, http.MethodGet, "/a/b")
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})
}

func TestRedirectConcurrency(t *testing.T) {
	deps := setupTestDB(t)
	mux := muxFor(deps.handler)

	const (
		code        = "conc-test"
		concurrency = 25
	)

	if _, err := deps.repo.Create(context.Background(), code, "https://example.com"); err != nil {
		t.Fatalf("create link: %v", err)
	}

	var (
		wg       sync.WaitGroup
		failures int
		mu       sync.Mutex
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := doRequest(mux, http.MethodGet, "/"+code)
			if rr.Code != http.StatusFound {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if failures != 0 {
		t.Fatalf("%d requests did not return 302", failures)
	}
	if c := clickCount(t, deps, code); c != concurrency {
		t.Errorf("click_count = %d, want %d", c, concurrency)
	}
}
