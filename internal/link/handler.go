package link

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

type Handler struct {
	service       *Service
	publicBaseURL string
}

func NewHandler(service *Service, publicBaseURL string) *Handler {
	return &Handler{
		service:       service,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

type createLinkRequest struct {
	DestinationURL string `json:"destination_url"`
	Code           string `json:"code"`
}

type patchLinkRequest struct {
	DestinationURL *string `json:"destination_url"`
	Status         *string `json:"status"`
}

type linkResponse struct {
	ID             string    `json:"id"`
	Code           string    `json:"code"`
	ShortURL       string    `json:"short_url"`
	DestinationURL string    `json:"destination_url"`
	Status         string    `json:"status"`
	ClickCount     int64     `json:"click_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) responseFor(l Link) linkResponse {
	return linkResponse{
		ID:             l.ID.String(),
		Code:           l.Code,
		ShortURL:       h.publicBaseURL + "/" + l.Code,
		DestinationURL: l.DestinationURL,
		Status:         l.Status,
		ClickCount:     l.ClickCount,
		CreatedAt:      l.CreatedAt,
		UpdatedAt:      l.UpdatedAt,
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// decodeStrictBody decodes a single JSON object with unknown fields rejected.
// Writes a 400 response and returns false on any violation.
func decodeStrictBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// Create handles POST /api/links.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	if !decodeStrictBody(w, r, &req) {
		return
	}

	l, err := h.service.Create(r.Context(), req.DestinationURL, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidDestinationURL):
			writeError(w, http.StatusBadRequest, "invalid destination_url")
		case errors.Is(err, ErrInvalidCode):
			writeError(w, http.StatusBadRequest, "invalid code")
		case errors.Is(err, ErrReservedCode):
			writeError(w, http.StatusBadRequest, "reserved code")
		case errors.Is(err, ErrCodeConflict):
			writeError(w, http.StatusConflict, "code already exists")
		default:
			log.Printf("create link: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, h.responseFor(l))
}

// List handles GET /api/links.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	links, err := h.service.List(r.Context())
	if err != nil {
		log.Printf("list links: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp := make([]linkResponse, 0, len(links))
	for _, l := range links {
		resp = append(resp, h.responseFor(l))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /api/links/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	l, err := h.service.Get(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrLinkNotFound):
			writeError(w, http.StatusNotFound, "link not found")
		default:
			log.Printf("get link %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, h.responseFor(l))
}

// Patch handles PATCH /api/links/{id}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req patchLinkRequest
	if !decodeStrictBody(w, r, &req) {
		return
	}
	if req.DestinationURL == nil && req.Status == nil {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	l, err := h.service.Update(r.Context(), id, req.DestinationURL, req.Status)
	if err != nil {
		switch {
		case errors.Is(err, ErrLinkNotFound):
			writeError(w, http.StatusNotFound, "link not found")
		case errors.Is(err, ErrInvalidDestinationURL):
			writeError(w, http.StatusBadRequest, "invalid destination_url")
		case errors.Is(err, ErrInvalidStatus):
			writeError(w, http.StatusBadRequest, "invalid status")
		default:
			log.Printf("patch link %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, h.responseFor(l))
}

// Redirect serves GET /{code}: looks up the active link, records the click,
// and issues a 302 redirect. Unknown and inactive links are indistinguishable
// (404). Redirect responses carry no JSON body.
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	destinationURL, err := h.service.Resolve(r.Context(), r.PathValue("code"))
	if err != nil {
		switch {
		case errors.Is(err, ErrLinkNotFound):
			writeError(w, http.StatusNotFound, "link not found")
		default:
			log.Printf("resolve link: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.Header().Set("Location", destinationURL)
	w.WriteHeader(http.StatusFound)
}
