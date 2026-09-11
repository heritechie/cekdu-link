package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func linkBody(t *testing.T, h http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}

func linkIDByCode(t *testing.T, deps *testDeps, code string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := deps.pool.QueryRow(context.Background(),
		"SELECT id FROM links WHERE code = $1", code,
	).Scan(&id); err != nil {
		t.Fatalf("get id for %q: %v", code, err)
	}
	return id
}

func rowUpdatedAt(t *testing.T, deps *testDeps, id uuid.UUID) time.Time {
	t.Helper()
	var ts time.Time
	if err := deps.pool.QueryRow(context.Background(),
		"SELECT updated_at FROM links WHERE id = $1", id,
	).Scan(&ts); err != nil {
		t.Fatalf("read updated_at for %s: %v", id, err)
	}
	return ts
}

func TestServiceManage(t *testing.T) {
	sample := Link{
		ID:             mustUUID(t, "765563fa-0ceb-4148-a615-0e10a29ec8e4"),
		Code:           "example-test",
		DestinationURL: "https://example.com",
		Status:         "active",
	}

	t.Run("list", func(t *testing.T) {
		repo := &fakeRepository{links: []Link{sample}}
		svc := NewService(repo)
		got, err := svc.List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(got) != 1 || got[0].Code != "example-test" {
			t.Errorf("List() = %+v", got)
		}
	})

	t.Run("get found", func(t *testing.T) {
		repo := &fakeRepository{getByID: sample}
		svc := NewService(repo)
		got, err := svc.Get(context.Background(), sample.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got.ID != sample.ID {
			t.Errorf("Get() id = %s", got.ID)
		}
	})

	t.Run("get not found", func(t *testing.T) {
		repo := &fakeRepository{getErr: ErrLinkNotFound}
		svc := NewService(repo)
		_, err := svc.Get(context.Background(), sample.ID)
		if !errors.Is(err, ErrLinkNotFound) {
			t.Fatalf("Get() error = %v, want ErrLinkNotFound", err)
		}
	})

	t.Run("update valid patch", func(t *testing.T) {
		repo := &fakeRepository{updated: sample}
		svc := NewService(repo)
		dest, status := "https://example.org", "inactive"
		got, err := svc.Update(context.Background(), sample.ID, &dest, &status)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if got.ID != sample.ID || repo.updateCalls != 1 {
			t.Errorf("Update() = %+v, calls = %d", got, repo.updateCalls)
		}
	})

	t.Run("update nil pointers still reach repo", func(t *testing.T) {
		repo := &fakeRepository{updated: sample}
		svc := NewService(repo)
		if _, err := svc.Update(context.Background(), sample.ID, nil, nil); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if repo.updateCalls != 1 {
			t.Errorf("updateCalls = %d, want 1", repo.updateCalls)
		}
	})

	t.Run("update invalid destination", func(t *testing.T) {
		repo := &fakeRepository{}
		svc := NewService(repo)
		bad := "ftp://example.com"
		_, err := svc.Update(context.Background(), sample.ID, &bad, nil)
		if !errors.Is(err, ErrInvalidDestinationURL) {
			t.Fatalf("Update() error = %v, want ErrInvalidDestinationURL", err)
		}
		if repo.updateCalls != 0 {
			t.Errorf("updateCalls = %d, want 0", repo.updateCalls)
		}
	})

	t.Run("update invalid status", func(t *testing.T) {
		repo := &fakeRepository{}
		svc := NewService(repo)
		bad := "paused"
		_, err := svc.Update(context.Background(), sample.ID, nil, &bad)
		if !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("Update() error = %v, want ErrInvalidStatus", err)
		}
		if repo.updateCalls != 0 {
			t.Errorf("updateCalls = %d, want 0", repo.updateCalls)
		}
	})
}

func TestHandlerGetInvalidUUIDDoesNotQuery(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	h := NewHandler(svc, testBaseURL)

	rr := doRequest(muxFor(h), http.MethodGet, "/api/links/not-a-uuid")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if repo.getCalls != 0 {
		t.Errorf("GetByID() called %d times with invalid uuid, want 0", repo.getCalls)
	}
}

func TestHandlerPatchUnit(t *testing.T) {
	newSvc := func() (*Handler, *fakeRepository) {
		repo := &fakeRepository{}
		return NewHandler(NewService(repo), testBaseURL), repo
	}

	id := "765563fa-0ceb-4148-a615-0e10a29ec8e4"

	t.Run("no fields to update", func(t *testing.T) {
		h, _ := newSvc()
		rr := linkBody(t, muxFor(h), http.MethodPatch, "/api/links/"+id, `{}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
		}
		var er errorResponse
		json.Unmarshal(rr.Body.Bytes(), &er)
		if er.Error != "no fields to update" {
			t.Errorf("error = %q", er.Error)
		}
	})

	t.Run("invalid id is a 400", func(t *testing.T) {
		h, repo := newSvc()
		rr := linkBody(t, muxFor(h), http.MethodPatch, "/api/links/not-a-uuid", `{"status":"inactive"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		if repo.updateCalls != 0 {
			t.Errorf("Update() called %d times, want 0", repo.updateCalls)
		}
	})

	t.Run("immutable fields are rejected as unknown", func(t *testing.T) {
		for _, field := range []string{"id", "code", "click_count", "created_at", "updated_at"} {
			h, repo := newSvc()
			rr := linkBody(t, muxFor(h), http.MethodPatch, "/api/links/"+id,
				`{"`+field+`":"`+strings.Repeat("a", 5)+`"}`)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("field %q: status = %d, want 400", field, rr.Code)
			}
			if repo.updateCalls != 0 {
				t.Errorf("field %q: Update() called %d times, want 0", field, repo.updateCalls)
			}
		}
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		h, repo := newSvc()
		rr := linkBody(t, muxFor(h), http.MethodPatch, "/api/links/"+id, `{"extra":1}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		if repo.updateCalls != 0 {
			t.Errorf("Update() called %d times, want 0", repo.updateCalls)
		}
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		h, repo := newSvc()
		rr := linkBody(t, muxFor(h), http.MethodPatch, "/api/links/"+id, `{"status":"paused"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		if repo.updateCalls != 0 {
			t.Errorf("Update() called %d times, want 0", repo.updateCalls)
		}
	})

	t.Run("valid patch returns updated link", func(t *testing.T) {
		h, _ := newSvc()
		rr := linkBody(t, muxFor(h), http.MethodPatch, "/api/links/"+id, `{"status":"inactive"}`)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
	})
}

func TestManageIntegration(t *testing.T) {
	deps := setupTestDB(t)
	mux := muxFor(deps.handler)

	t.Run("list returns links newest first", func(t *testing.T) {
		for i, code := range []string{"list-a", "list-b", "list-c"} {
			dest := "https://example.com/" + code
			if _, err := deps.pool.Exec(context.Background(),
				`INSERT INTO links (code, destination_url, created_at, updated_at)
				 VALUES ($1, $2, now() - make_interval(secs => $3), now())`,
				code, dest, float64(30-i*10)); err != nil {
				t.Fatalf("insert %q: %v", code, err)
			}
		}

		rr := doRequest(mux, http.MethodGet, "/api/links")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		var list []linkResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(list) < 3 {
			t.Fatalf("len(list) = %d, want >= 3", len(list))
		}
		codes := []string{list[0].Code, list[1].Code, list[2].Code}
		want := []string{"list-c", "list-b", "list-a"}
		for i := range want {
			if codes[i] != want[i] {
				t.Errorf("order = %v, want %v", codes, want)
				break
			}
		}
	})

	var updatedID uuid.UUID

	t.Run("get by id", func(t *testing.T) {
		rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com","code":"get-me"}`)
		resp := decodeResponse(t, rr)
		id := resp.ID

		got := doRequest(mux, http.MethodGet, "/api/links/"+id)
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", got.Code, got.Body.String())
		}
		var out linkResponse
		json.Unmarshal(got.Body.Bytes(), &out)
		if out.ID != id || out.Code != "get-me" || out.DestinationURL != "https://example.com" {
			t.Errorf("response = %+v", out)
		}
		updatedID = mustUUID(t, id)
	})

	t.Run("get unknown id is 404", func(t *testing.T) {
		rr := doRequest(mux, http.MethodGet, "/api/links/"+uuid.New().String())
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body=%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("patch updates destination and bumps updated_at", func(t *testing.T) {
		before := rowUpdatedAt(t, deps, updatedID)
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/"+updatedID.String(),
			`{"destination_url":"https://example.org/new"}`)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		var out linkResponse
		json.Unmarshal(rr.Body.Bytes(), &out)
		if out.DestinationURL != "https://example.org/new" {
			t.Errorf("destination_url = %q", out.DestinationURL)
		}
		if out.Code != "get-me" {
			t.Errorf("code changed to %q, want get-me (immutable)", out.Code)
		}
		after := rowUpdatedAt(t, deps, updatedID)
		if !after.After(before) {
			t.Errorf("updated_at not bumped: before=%v after=%v", before, after)
		}
	})

	t.Run("patch with no actual change keeps updated_at", func(t *testing.T) {
		before := rowUpdatedAt(t, deps, updatedID)
		time.Sleep(5 * time.Millisecond)
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/"+updatedID.String(),
			`{"destination_url":"https://example.org/new"}`)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		after := rowUpdatedAt(t, deps, updatedID)
		if after != before {
			t.Errorf("updated_at changed on no-op patch: before=%v after=%v", before, after)
		}
	})

	t.Run("patch preserves click_count", func(t *testing.T) {
		// bump clicks via redirect, then patch, then verify counts preserved
		if _, _, err := deps.repo.IncrementActiveClick(context.Background(), "get-me"); err != nil {
			t.Fatalf("increment click: %v", err)
		}
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/"+updatedID.String(),
			`{"destination_url":"https://example.org/preserved"}`)
		var out linkResponse
		json.Unmarshal(rr.Body.Bytes(), &out)
		if out.ClickCount != 1 {
			t.Errorf("click_count = %d, want 1", out.ClickCount)
		}
	})

	t.Run("patch unknown id is 404", func(t *testing.T) {
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/"+uuid.New().String(), `{"status":"inactive"}`)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})

	t.Run("patch invalid id is 400", func(t *testing.T) {
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/not-a-uuid", `{"status":"inactive"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
		}
		var er errorResponse
		json.Unmarshal(rr.Body.Bytes(), &er)
		if er.Error != "invalid id" {
			t.Errorf("error = %q", er.Error)
		}
	})

	t.Run("patch malformed body is 400", func(t *testing.T) {
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/"+updatedID.String(), `{"status":`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("patch trailing json is 400", func(t *testing.T) {
		rr := linkBody(t, mux, http.MethodPatch, "/api/links/"+updatedID.String(),
			`{"status":"active"} {"status":"inactive"}`)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("unsupported methods on link are 405", func(t *testing.T) {
		for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPost} {
			rr := linkBody(t, mux, method, "/api/links/"+updatedID.String(), `{}`)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("method %s: status = %d, want 405", method, rr.Code)
			}
		}
	})
}

func TestReactivation(t *testing.T) {
	deps := setupTestDB(t)
	mux := muxFor(deps.handler)

	rr := postJSON(t, deps.handler, `{"destination_url":"https://example.com","code":"react-aaa"}`)
	id := mustUUID(t, decodeResponse(t, rr).ID)

	if c := clickCount(t, deps, "react-aaa"); c != 0 {
		t.Fatalf("initial click_count = %d", c)
	}
	if rr := doRequest(mux, http.MethodGet, "/react-aaa"); rr.Code != http.StatusFound {
		t.Fatalf("active redirect = %d, want 302", rr.Code)
	}
	if c := clickCount(t, deps, "react-aaa"); c != 1 {
		t.Fatalf("click_count after active = %d, want 1", c)
	}

	rr = linkBody(t, mux, http.MethodPatch, "/api/links/"+id.String(), `{"status":"inactive"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("deactivate status = %d (body=%s)", rr.Code, rr.Body.String())
	}
	if rr := doRequest(mux, http.MethodGet, "/react-aaa"); rr.Code != http.StatusNotFound {
		t.Fatalf("inactive redirect = %d, want 404", rr.Code)
	}
	if c := clickCount(t, deps, "react-aaa"); c != 1 {
		t.Fatalf("click_count changed on inactive = %d, want 1", c)
	}

	rr = linkBody(t, mux, http.MethodPatch, "/api/links/"+id.String(), `{"status":"active"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("reactivate status = %d (body=%s)", rr.Code, rr.Body.String())
	}
	if rr := doRequest(mux, http.MethodGet, "/react-aaa"); rr.Code != http.StatusFound {
		t.Fatalf("reactivated redirect = %d, want 302", rr.Code)
	}
	if c := clickCount(t, deps, "react-aaa"); c != 2 {
		t.Fatalf("click_count after reactivation = %d, want 2 (must not reset)", c)
	}
}
