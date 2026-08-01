package main

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "net/http"
    "time"

    "github.com/go-webauthn/webauthn/protocol"
    "github.com/go-webauthn/webauthn/webauthn"
)

var webAuthn *webauthn.WebAuthn

func init() {
    var err error
    webAuthn, err = webauthn.New(&webauthn.Config{
        RPDisplayName: "Assylum DeFi Aggregator",
        RPID:          "localhost",
        RPOrigins:     []string{"http://localhost:8080"},
    })
    if err != nil {
        panic(err)
    }
}

type WebAuthnUser struct {
    ID          []byte
    Name        string
    DisplayName string
    Credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte                         { return u.ID }
func (u *WebAuthnUser) WebAuthnName() string                       { return u.Name }
func (u *WebAuthnUser) WebAuthnDisplayName() string                { return u.DisplayName }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

func generateSecureToken() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.RawURLEncoding.EncodeToString(b)
}

func registerOptionsHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email string `json:"email"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
        http.Error(w, "Email is required", http.StatusBadRequest)
        return
    }

    existingUser, _ := getUserByEmail(req.Email)
    if existingUser != nil {
        http.Error(w, "User already exists", http.StatusConflict)
        return
    }

    userID := make([]byte, 16)
    rand.Read(userID)

    user := &WebAuthnUser{
        ID:          userID,
        Name:        req.Email,
        DisplayName: req.Email,
    }

    options, sessionData, err := webAuthn.BeginRegistration(user)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    sessionID := generateSecureToken()
    expiresAt := time.Now().Add(5 * time.Minute)

    savePendingRegistration(sessionID, userID, req.Email, sessionData, expiresAt)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "sessionID": sessionID,
        "publicKey": options,
    })
}

func registerVerifyHandler(w http.ResponseWriter, r *http.Request) {
    sessionID := r.URL.Query().Get("sessionID")
    if sessionID == "" {
        http.Error(w, "Missing sessionID", http.StatusBadRequest)
        return
    }

    pending, err := getPendingRegistration(sessionID)
    if err != nil || time.Now().After(pending.ExpiresAt) {
        http.Error(w, "Session expired", http.StatusBadRequest)
        return
    }

    parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
    if err != nil {
        http.Error(w, "Invalid credential", http.StatusBadRequest)
        return
    }

    user := &WebAuthnUser{
        ID:          pending.UserID,
        Name:        pending.Email,
        DisplayName: pending.Email,
    }

    credential, err := webAuthn.CreateCredential(user, pending.SessionData, parsed)
    if err != nil {
        http.Error(w, "Verification failed", http.StatusBadRequest)
        return
    }

    saveUser(pending.UserID, pending.Email)
    saveCredential(pending.UserID, credential)
    deletePendingRegistration(sessionID)

    token := generateSecureToken()
    expiresAt := time.Now().Add(24 * time.Hour)
    saveSession(token, pending.UserID, expiresAt)

    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    token,
        Path:     "/",
        Expires:  expiresAt,
        HttpOnly: true,
        Secure:   false,
        SameSite: http.SameSiteLaxMode,
    })

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func loginOptionsHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email string `json:"email"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
        http.Error(w, "Email is required", http.StatusBadRequest)
        return
    }

    userRecord, err := getUserByEmail(req.Email)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    creds, err := getCredentials(userRecord.ID)
    if err != nil || len(creds) == 0 {
        http.Error(w, "No credentials", http.StatusBadRequest)
        return
    }

    user := &WebAuthnUser{
        ID:          userRecord.ID,
        Name:        userRecord.Email,
        DisplayName: userRecord.Email,
        Credentials: creds,
    }

    options, sessionData, err := webAuthn.BeginLogin(user)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    sessionID := generateSecureToken()
    expiresAt := time.Now().Add(5 * time.Minute)

    savePendingLogin(sessionID, userRecord.ID, sessionData, expiresAt)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "sessionID": sessionID,
        "publicKey": options,
    })
}

func loginVerifyHandler(w http.ResponseWriter, r *http.Request) {
    sessionID := r.URL.Query().Get("sessionID")
    if sessionID == "" {
        http.Error(w, "Missing sessionID", http.StatusBadRequest)
        return
    }

    pending, err := getPendingLogin(sessionID)
    if err != nil || time.Now().After(pending.ExpiresAt) {
        http.Error(w, "Session expired", http.StatusBadRequest)
        return
    }

    userRecord, err := getUserByID(pending.UserID)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    creds, _ := getCredentials(userRecord.ID)

    user := &WebAuthnUser{
        ID:          userRecord.ID,
        Name:        userRecord.Email,
        DisplayName: userRecord.Email,
        Credentials: creds,
    }

    parsed, err := protocol.ParseCredentialRequestResponseBody(r.Body)
    if err != nil {
        http.Error(w, "Invalid assertion", http.StatusBadRequest)
        return
    }

    credential, err := webAuthn.ValidateLogin(user, pending.SessionData, parsed)
    if err != nil {
        http.Error(w, "Authentication failed", http.StatusUnauthorized)
        return
    }

    updateCredentialSignCount(userRecord.ID, credential)
    deletePendingLogin(sessionID)

    token := generateSecureToken()
    expiresAt := time.Now().Add(24 * time.Hour)
    saveSession(token, userRecord.ID, expiresAt)

    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    token,
        Path:     "/",
        Expires:  expiresAt,
        HttpOnly: true,
        Secure:   false,
        SameSite: http.SameSiteLaxMode,
    })

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "signed_in"})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie("session_token")
    if err == nil {
        deleteSession(cookie.Value)
    }

    http.SetCookie(w, &http.Cookie{
        Name:     "session_token",
        Value:    "",
        Path:     "/",
        Expires:  time.Unix(0, 0),
        HttpOnly: true,
    })

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie("session_token")
    if err != nil {
        http.Error(w, "Not authenticated", http.StatusUnauthorized)
        return
    }

    userID, err := getSessionUser(cookie.Value)
    if err != nil {
        http.Error(w, "Session expired", http.StatusUnauthorized)
        return
    }

    user, err := getUserByID(userID)
    if err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "email": user.Email,
        "id":    base64.RawURLEncoding.EncodeToString(user.ID),
    })
}
