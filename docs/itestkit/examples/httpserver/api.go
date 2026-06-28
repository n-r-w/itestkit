// Package httpserverexample shows JSONC-driven tests for an in-process HTTP API.
package httpserverexample

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/n-r-w/itestkit"
)

const (
	contentTypeHeader        = "Content-Type"
	invalidLoginRequestError = "invalid login request"
	jsonContentType          = "application/json"
	methodNotAllowedError    = "method not allowed"
	sessionCookieName        = "session_id"
	sessionCookiePath        = "/"
	sessionID                = "session-1"
)

// userAccount is the account accepted by the example API.
type userAccount struct {
	UserID   string
	Email    string
	Password string
}

// loginRequest is the JSON payload accepted by POST /login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the JSON response returned after successful login.
type loginResponse struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

// meResponse is the JSON response returned for the authenticated user.
type meResponse struct {
	UserID        string `json:"user_id"`
	Email         string `json:"email"`
	Authenticated bool   `json:"authenticated"`
}

// errorResponse is the stable JSON error shape used by the example API.
type errorResponse struct {
	Error string `json:"error"`
}

// apiHandler serves a small session-based HTTP API for the example cases.
type apiHandler struct {
	account  userAccount
	sessions map[string]userAccount
}

// compileTimeAPIHandlerCheck verifies the implementation of http.Handler.
var _ http.Handler = (*apiHandler)(nil)

// newAPIHandler creates the HTTP API under test.
func newAPIHandler() http.Handler {
	return &apiHandler{
		account: userAccount{
			UserID:   "user-1",
			Email:    "admin@example.test",
			Password: "correct-password",
		},
		sessions: make(map[string]userAccount),
	}
}

// ServeHTTP routes example API requests by path.
func (handler *apiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/login":
		handler.handleLogin(writer, request)
	case "/me":
		handler.handleMe(writer, request)
	default:
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "endpoint not found"})
	}
}

// handleLogin validates credentials, stores a session, and sends Set-Cookie.
func (handler *apiHandler) handleLogin(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeJSON(writer, http.StatusMethodNotAllowed, errorResponse{Error: methodNotAllowedError})
		return
	}

	payload, ok := decodeLoginRequest(writer, request)
	if !ok {
		return
	}
	if payload.Email != handler.account.Email || payload.Password != handler.account.Password {
		writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
		return
	}

	handler.sessions[sessionID] = handler.account
	cookie := new(http.Cookie)
	cookie.Name = sessionCookieName
	cookie.Value = sessionID
	cookie.Path = sessionCookiePath
	cookie.HttpOnly = true
	cookie.Secure = true
	cookie.SameSite = http.SameSiteLaxMode
	http.SetCookie(writer, cookie)
	writer.Header().Set("X-Session-Issued", "true")
	writeJSON(writer, http.StatusOK, loginResponse{UserID: handler.account.UserID, Email: handler.account.Email})
}

// handleMe returns the current user when the request contains a known session cookie.
func (handler *apiHandler) handleMe(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, errorResponse{Error: methodNotAllowedError})
		return
	}

	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "session cookie is required"})
		return
	}
	account, exists := handler.sessions[cookie.Value]
	if !exists {
		writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "session is unknown"})
		return
	}

	writeJSON(writer, http.StatusOK, meResponse{
		UserID:        account.UserID,
		Email:         account.Email,
		Authenticated: true,
	})
}

// decodeLoginRequest strictly decodes a login request and writes a 400 response on invalid input.
func decodeLoginRequest(writer http.ResponseWriter, request *http.Request) (loginRequest, bool) {
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: invalidLoginRequestError})
		return loginRequest{}, false
	}

	payload := loginRequest{Email: "", Password: ""}
	decodeErr := itestkit.DecodeStrictJSON(rawBody, &payload)
	if decodeErr != nil {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: invalidLoginRequestError})
		return loginRequest{}, false
	}
	return payload, true
}

// writeJSON writes one stable JSON response shape for fixture comparison.
func writeJSON(writer http.ResponseWriter, status int, response any) {
	writer.Header().Set(contentTypeHeader, jsonContentType)
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		return
	}
}
