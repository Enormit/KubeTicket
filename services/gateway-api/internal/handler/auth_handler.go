package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
)

// AuthHandler exposes registration, login, logout, and session endpoints.
type AuthHandler struct {
	svc          *service.AuthService
	jwtTTL       time.Duration
	cookieSecure bool
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(svc *service.AuthService, jwtTTL time.Duration, cookieSecure bool) *AuthHandler {
	return &AuthHandler{svc: svc, jwtTTL: jwtTTL, cookieSecure: cookieSecure}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_json", "malformed JSON request")
		return
	}

	u, tok, err := h.svc.Register(r.Context(), req.Email, req.Password, req.FullName)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			middleware.WriteError(w, http.StatusBadRequest, "invalid_input", "valid email, full_name, and password (>= 6 chars) are required")
		case errors.Is(err, repo.ErrEmailExists):
			middleware.WriteError(w, http.StatusConflict, "email_exists", "email is already registered")
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to register user")
		}
		return
	}

	middleware.SetAuthCookie(w, tok, h.jwtTTL, h.cookieSecure)
	middleware.WriteJSON(w, http.StatusCreated, map[string]any{
		"user":  u,
		"token": tok,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_json", "malformed JSON request")
		return
	}

	u, tok, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			middleware.WriteError(w, http.StatusBadRequest, "invalid_input", "email and password are required")
		case errors.Is(err, service.ErrInvalidCredentials):
			middleware.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to login")
		}
		return
	}

	middleware.SetAuthCookie(w, tok, h.jwtTTL, h.cookieSecure)
	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"user":  u,
		"token": tok,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.ClearAuthCookie(w, h.cookieSecure)
	middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	u, err := h.svc.GetMe(r.Context(), authUser.ID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load profile")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"user": u})
}
