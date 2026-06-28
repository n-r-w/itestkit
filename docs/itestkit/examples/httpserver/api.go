// Package httpserverexample shows JSONC-driven tests for an in-process HTTP API.
package httpserverexample

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/n-r-w/itestkit"
)

const (
	contentTypeHeader         = "Content-Type"
	csrfCookieName            = "csrf_token"
	csrfHeaderName            = "X-CSRF-Token"
	csrfToken                 = "csrf-1"
	invalidLoginRequestError  = "invalid login request"
	invalidUpdateRequestError = "invalid update request"
	jsonContentType           = "application/json"
	methodNotAllowedError     = "method not allowed"
	reportContentType         = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	reportXLSXBody            = "PK\x03\x04report"
	sessionCookieName         = "session_id"
	sessionCookiePath         = "/"
	sessionID                 = "session-1"
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

// updateEmailRequest is the JSON payload accepted by POST /profile/email.
type updateEmailRequest struct {
	Email string `json:"email"`
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
	case "/profile/email":
		handler.handleProfileEmail(writer, request)
	case "/report.xlsx":
		handler.handleReport(writer, request)
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
	sessionCookie := new(http.Cookie)
	sessionCookie.Name = sessionCookieName
	sessionCookie.Value = sessionID
	sessionCookie.Path = sessionCookiePath
	sessionCookie.HttpOnly = true
	sessionCookie.Secure = true
	sessionCookie.SameSite = http.SameSiteLaxMode
	http.SetCookie(writer, sessionCookie)

	csrfCookie := new(http.Cookie)
	csrfCookie.Name = csrfCookieName
	csrfCookie.Value = csrfToken
	csrfCookie.Path = sessionCookiePath
	csrfCookie.HttpOnly = true
	csrfCookie.Secure = true
	csrfCookie.SameSite = http.SameSiteLaxMode
	http.SetCookie(writer, csrfCookie)

	writer.Header().Set("X-Session-Issued", "true")
	writeJSON(writer, http.StatusOK, loginResponse{UserID: handler.account.UserID, Email: handler.account.Email})
}

// handleMe returns the current user when the request contains a known session cookie.
func (handler *apiHandler) handleMe(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, errorResponse{Error: methodNotAllowedError})
		return
	}

	account, _, ok := handler.authenticatedAccount(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, meResponse{
		UserID:        account.UserID,
		Email:         account.Email,
		Authenticated: true,
	})
}

// handleProfileEmail updates the authenticated user's email when the CSRF token matches the stored cookie.
func (handler *apiHandler) handleProfileEmail(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeJSON(writer, http.StatusMethodNotAllowed, errorResponse{Error: methodNotAllowedError})
		return
	}

	account, sessionCookie, ok := handler.authenticatedAccount(writer, request)
	if !ok {
		return
	}
	if !validCSRF(request) {
		writeJSON(writer, http.StatusForbidden, errorResponse{Error: "csrf token is invalid"})
		return
	}

	payload, ok := decodeUpdateEmailRequest(writer, request)
	if !ok {
		return
	}
	account.Email = payload.Email
	handler.sessions[sessionCookie.Value] = account
	writeJSON(writer, http.StatusOK, meResponse{
		UserID:        account.UserID,
		Email:         account.Email,
		Authenticated: true,
	})
}

// handleReport returns a binary report body that needs custom fixture normalization.
func (handler *apiHandler) handleReport(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, errorResponse{Error: methodNotAllowedError})
		return
	}

	writer.Header().Set(contentTypeHeader, reportContentType)
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte(reportXLSXBody)); err != nil {
		return
	}
}

// authenticatedAccount returns the account bound to the request session cookie.
func (handler *apiHandler) authenticatedAccount(
	writer http.ResponseWriter,
	request *http.Request,
) (userAccount, *http.Cookie, bool) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "session cookie is required"})
		return userAccount{}, nil, false
	}
	account, exists := handler.sessions[cookie.Value]
	if !exists {
		writeJSON(writer, http.StatusUnauthorized, errorResponse{Error: "session is unknown"})
		return userAccount{}, nil, false
	}
	return account, cookie, true
}

// validCSRF accepts only requests where the configured header matches the CSRF cookie value.
func validCSRF(request *http.Request) bool {
	csrfCookie, err := request.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	return request.Header.Get(csrfHeaderName) == csrfCookie.Value
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

// decodeUpdateEmailRequest strictly decodes an email update request and writes a 400 response on invalid input.
func decodeUpdateEmailRequest(writer http.ResponseWriter, request *http.Request) (updateEmailRequest, bool) {
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: invalidUpdateRequestError})
		return updateEmailRequest{}, false
	}

	payload := updateEmailRequest{Email: ""}
	decodeErr := itestkit.DecodeStrictJSON(rawBody, &payload)
	if decodeErr != nil {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: invalidUpdateRequestError})
		return updateEmailRequest{}, false
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
