package main

import (
    "database/sql"
    "encoding/json"
    "errors"
    "time"

    "github.com/go-webauthn/webauthn/webauthn"
    _ "modernc.org/sqlite"
)

var db *sql.DB

type UserRecord struct {
    ID    []byte
    Email string
}

type PendingRegistration struct {
    UserID      []byte
    Email       string
    SessionData webauthn.SessionData
    ExpiresAt   time.Time
}

type PendingLogin struct {
    UserID      []byte
    SessionData webauthn.SessionData
    ExpiresAt   time.Time
}

func initDB() {
    var err error
    db, err = sql.Open("sqlite", "/app/data/data.db")
    if err != nil {
        panic(err)
    }

    queries := []string{
        `CREATE TABLE IF NOT EXISTS users (
            id BLOB PRIMARY KEY,
            email TEXT UNIQUE NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );`,
        `CREATE TABLE IF NOT EXISTS credentials (
            id BLOB PRIMARY KEY,
            user_id BLOB NOT NULL,
            public_key BLOB NOT NULL,
            attestation_type TEXT,
            transport TEXT,
            flags INTEGER,
            authenticator BLOB,
            sign_count INTEGER DEFAULT 0,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(user_id) REFERENCES users(id)
        );`,
        `CREATE TABLE IF NOT EXISTS pending_registrations (
            session_id TEXT PRIMARY KEY,
            user_id BLOB NOT NULL,
            email TEXT NOT NULL,
            session_data TEXT NOT NULL,
            expires_at DATETIME NOT NULL
        );`,
        `CREATE TABLE IF NOT EXISTS pending_logins (
            session_id TEXT PRIMARY KEY,
            user_id BLOB NOT NULL,
            session_data TEXT NOT NULL,
            expires_at DATETIME NOT NULL
        );`,
        `CREATE TABLE IF NOT EXISTS sessions (
            token TEXT PRIMARY KEY,
            user_id BLOB NOT NULL,
            expires_at DATETIME NOT NULL
        );`,
    }

    for _, q := range queries {
        if _, err := db.Exec(q); err != nil {
            panic(err)
        }
    }
}

func savePendingRegistration(sessionID string, userID []byte, email string, data *webauthn.SessionData, expiresAt time.Time) error {
    dataJSON, err := json.Marshal(data)
    if err != nil {
        return err
    }
    _, err = db.Exec(`INSERT INTO pending_registrations (session_id, user_id, email, session_data, expires_at) VALUES (?, ?, ?, ?, ?)`,
        sessionID, userID, email, string(dataJSON), expiresAt)
    return err
}

func getPendingRegistration(sessionID string) (*PendingRegistration, error) {
    var p PendingRegistration
    var dataJSON string
    var expiresStr string
    err := db.QueryRow(`SELECT user_id, email, session_data, expires_at FROM pending_registrations WHERE session_id = ?`, sessionID).
        Scan(&p.UserID, &p.Email, &dataJSON, &expiresStr)
    if err != nil {
        return nil, err
    }
    p.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresStr)
    if err := json.Unmarshal([]byte(dataJSON), &p.SessionData); err != nil {
        return nil, err
    }
    return &p, nil
}

func deletePendingRegistration(sessionID string) {
    db.Exec(`DELETE FROM pending_registrations WHERE session_id = ?`, sessionID)
}

func savePendingLogin(sessionID string, userID []byte, data *webauthn.SessionData, expiresAt time.Time) error {
    dataJSON, err := json.Marshal(data)
    if err != nil {
        return err
    }
    _, err = db.Exec(`INSERT INTO pending_logins (session_id, user_id, session_data, expires_at) VALUES (?, ?, ?, ?)`,
        sessionID, userID, string(dataJSON), expiresAt)
    return err
}

func getPendingLogin(sessionID string) (*PendingLogin, error) {
    var p PendingLogin
    var dataJSON string
    var expiresStr string
    err := db.QueryRow(`SELECT user_id, session_data, expires_at FROM pending_logins WHERE session_id = ?`, sessionID).
        Scan(&p.UserID, &dataJSON, &expiresStr)
    if err != nil {
        return nil, err
    }
    p.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresStr)
    if err := json.Unmarshal([]byte(dataJSON), &p.SessionData); err != nil {
        return nil, err
    }
    return &p, nil
}

func deletePendingLogin(sessionID string) {
    db.Exec(`DELETE FROM pending_logins WHERE session_id = ?`, sessionID)
}

func saveUser(userID []byte, email string) error {
    _, err := db.Exec(`INSERT INTO users (id, email) VALUES (?, ?)`, userID, email)
    return err
}

func getUserByEmail(email string) (*UserRecord, error) {
    var u UserRecord
    err := db.QueryRow(`SELECT id, email FROM users WHERE email = ?`, email).Scan(&u.ID, &u.Email)
    if err != nil {
        return nil, err
    }
    return &u, nil
}

func getUserByID(id []byte) (*UserRecord, error) {
    var u UserRecord
    err := db.QueryRow(`SELECT id, email FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Email)
    if err != nil {
        return nil, err
    }
    return &u, nil
}

func saveCredential(userID []byte, c *webauthn.Credential) error {
    transportJSON, _ := json.Marshal(c.Transport)
    _, err := db.Exec(`INSERT INTO credentials (id, user_id, public_key, attestation_type, transport, flags, authenticator, sign_count)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        c.ID, userID, c.PublicKey, c.AttestationType, string(transportJSON), c.Flags, c.Authenticator, c.Authenticator.SignCount)
    return err
}

func getCredentials(userID []byte) ([]webauthn.Credential, error) {
    rows, err := db.Query(`SELECT id, public_key, attestation_type, transport, flags, authenticator, sign_count FROM credentials WHERE user_id = ?`, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var creds []webauthn.Credential
    for rows.Next() {
        var c webauthn.Credential
        var transport string
        var authData []byte
        err := rows.Scan(&c.ID, &c.PublicKey, &c.AttestationType, &transport, &c.Flags, &authData, &c.Authenticator.SignCount)
        if err != nil {
            return nil, err
        }
        json.Unmarshal([]byte(transport), &c.Transport)
        creds = append(creds, c)
    }
    return creds, nil
}

func updateCredentialSignCount(userID []byte, c *webauthn.Credential) error {
    _, err := db.Exec(`UPDATE credentials SET sign_count = ? WHERE id = ? AND user_id = ?`,
        c.Authenticator.SignCount, c.ID, userID)
    return err
}

func saveSession(token string, userID []byte, expiresAt time.Time) {
    db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, expiresAt)
}

func getSessionUser(token string) ([]byte, error) {
    var userID []byte
    var expiresStr string
    err := db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token = ?`, token).Scan(&userID, &expiresStr)
    if err != nil {
        return nil, err
    }
    expiresAt, _ := time.Parse("2006-01-02 15:04:05", expiresStr)
    if time.Now().After(expiresAt) {
        return nil, errors.New("session expired")
    }
    return userID, nil
}

func deleteSession(token string) {
    db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
}
