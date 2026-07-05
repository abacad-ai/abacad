// Package store is the SQLite persistence layer: accounts, sessions, devices,
// and per-account MCP tokens. Pure-Go driver (modernc.org/sqlite, no CGO) so the
// server is a single static-ish binary that's trivial to deploy on one host.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"abacad/internal/auth"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Token prefixes (cosmetic; the whole string is hashed).
const (
	deviceTokenPrefix = "abd_dev"
	mcpTokenPrefix    = "abd_mcp"
)

// ErrNotFound is returned by lookups that match no row. ErrEmailTaken is returned
// when registering an email that already exists.
var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already registered")
)

// Store wraps the database handle.
type Store struct{ db *sql.DB }

// Models. Timestamps are unix seconds; the API layer formats them.
type Account struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    int64
}

type Device struct {
	ID        string
	AccountID string
	Name      string
	Platform  string
	CreatedAt int64
	LastSeen  int64
}

type MCPTokenInfo struct {
	Exists    bool
	CreatedAt int64
	LastUsed  int64
}

// Open opens (creating if needed) the SQLite database at path and runs
// migrations. WAL + busy_timeout + foreign keys are set via the DSN.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc/sqlite is safe for concurrent use, but a single writer avoids
	// SQLITE_BUSY churn at this scale.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	sqlBytes, err := migrationsFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		return err
	}
	_, err = s.db.Exec(string(sqlBytes))
	return err
}

func now() int64 { return time.Now().Unix() }

// --- Accounts ---

// CreateAccount registers a new account. Returns ErrEmailTaken on a duplicate
// email.
func (s *Store) CreateAccount(email, passwordHash string) (Account, error) {
	a := Account{ID: auth.NewID("acc"), Email: email, PasswordHash: passwordHash, CreatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO accounts(id,email,password_hash,created_at) VALUES(?,?,?,?)`,
		a.ID, a.Email, a.PasswordHash, a.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return Account{}, ErrEmailTaken
		}
		return Account{}, err
	}
	return a, nil
}

func (s *Store) AccountByEmail(email string) (Account, error) {
	return s.scanAccount(s.db.QueryRow(
		`SELECT id,email,password_hash,created_at FROM accounts WHERE email=?`, email))
}

func (s *Store) AccountByID(id string) (Account, error) {
	return s.scanAccount(s.db.QueryRow(
		`SELECT id,email,password_hash,created_at FROM accounts WHERE id=?`, id))
}

func (s *Store) scanAccount(row *sql.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// --- Sessions ---

// CreateSession issues a web session valid for ttl and returns its opaque id
// (the cookie value).
func (s *Store) CreateSession(accountID, userAgent string, ttl time.Duration) (string, error) {
	id := auth.NewSecret("abd_sess")
	created := now()
	expires := time.Now().Add(ttl).Unix()
	_, err := s.db.Exec(`INSERT INTO sessions(id,account_id,created_at,expires_at,user_agent) VALUES(?,?,?,?,?)`,
		id, accountID, created, expires, userAgent)
	return id, err
}

// AccountBySession returns the account for a non-expired session id.
func (s *Store) AccountBySession(sessionID string) (Account, error) {
	row := s.db.QueryRow(
		`SELECT a.id,a.email,a.password_hash,a.created_at
		   FROM sessions s JOIN accounts a ON a.id=s.account_id
		  WHERE s.id=? AND s.expires_at > ?`, sessionID, now())
	return s.scanAccount(row)
}

func (s *Store) DeleteSession(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, sessionID)
	return err
}

// --- Devices ---

// CreateDevice adds a device to an account and returns it along with the
// plaintext device token (shown once; only its hash is stored).
func (s *Store) CreateDevice(accountID, name string) (Device, string, error) {
	token := auth.NewSecret(deviceTokenPrefix)
	d := Device{ID: auth.NewID("dev"), AccountID: accountID, Name: name, CreatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO devices(id,account_id,name,token_hash,platform,created_at,last_seen)
		VALUES(?,?,?,?,?,?,0)`, d.ID, d.AccountID, d.Name, auth.HashToken(token), d.Platform, d.CreatedAt)
	if err != nil {
		return Device{}, "", err
	}
	return d, token, nil
}

// DeviceByTokenHash resolves a device token (already hashed) to its device.
func (s *Store) DeviceByTokenHash(tokenHash string) (Device, error) {
	return s.scanDevice(s.db.QueryRow(
		`SELECT id,account_id,name,platform,created_at,last_seen FROM devices WHERE token_hash=?`, tokenHash))
}

// DeviceOwnedBy returns a device only if it belongs to accountID.
func (s *Store) DeviceOwnedBy(deviceID, accountID string) (Device, error) {
	return s.scanDevice(s.db.QueryRow(
		`SELECT id,account_id,name,platform,created_at,last_seen FROM devices WHERE id=? AND account_id=?`,
		deviceID, accountID))
}

// DevicesByAccount lists an account's devices, most-recently-active first.
func (s *Store) DevicesByAccount(accountID string) ([]Device, error) {
	rows, err := s.db.Query(
		`SELECT id,account_id,name,platform,created_at,last_seen FROM devices
		  WHERE account_id=? ORDER BY last_seen DESC, created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) RenameDevice(deviceID, accountID, name string) error {
	return s.affect(s.db.Exec(`UPDATE devices SET name=? WHERE id=? AND account_id=?`, name, deviceID, accountID))
}

func (s *Store) DeleteDevice(deviceID, accountID string) error {
	return s.affect(s.db.Exec(`DELETE FROM devices WHERE id=? AND account_id=?`, deviceID, accountID))
}

// RotateDeviceToken issues a fresh device token, invalidating the old one.
func (s *Store) RotateDeviceToken(deviceID, accountID string) (string, error) {
	token := auth.NewSecret(deviceTokenPrefix)
	if err := s.affect(s.db.Exec(`UPDATE devices SET token_hash=? WHERE id=? AND account_id=?`,
		auth.HashToken(token), deviceID, accountID)); err != nil {
		return "", err
	}
	return token, nil
}

// TouchDevice bumps last_seen to now (called when a device connects/replies).
func (s *Store) TouchDevice(deviceID string) {
	_, _ = s.db.Exec(`UPDATE devices SET last_seen=? WHERE id=?`, now(), deviceID)
}

func (s *Store) scanDevice(row *sql.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.AccountID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

// --- MCP tokens (one per account) ---

// AccountByMCPTokenHash resolves an MCP bearer token (already hashed) to its
// account and records last_used.
func (s *Store) AccountByMCPTokenHash(tokenHash string) (Account, error) {
	var accountID string
	err := s.db.QueryRow(`SELECT account_id FROM account_mcp_tokens WHERE token_hash=?`, tokenHash).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	_, _ = s.db.Exec(`UPDATE account_mcp_tokens SET last_used=? WHERE token_hash=?`, now(), tokenHash)
	return s.AccountByID(accountID)
}

// RotateMCPToken creates or replaces the account's MCP token and returns the
// plaintext (shown once).
func (s *Store) RotateMCPToken(accountID string) (string, error) {
	token := auth.NewSecret(mcpTokenPrefix)
	_, err := s.db.Exec(`INSERT INTO account_mcp_tokens(id,account_id,token_hash,created_at,last_used)
		VALUES(?,?,?,?,0)
		ON CONFLICT(account_id) DO UPDATE SET token_hash=excluded.token_hash, created_at=excluded.created_at, last_used=0`,
		auth.NewID("mcptok"), accountID, auth.HashToken(token), now())
	if err != nil {
		return "", err
	}
	return token, nil
}

// MCPToken returns metadata about the account's MCP token (never the secret).
func (s *Store) MCPToken(accountID string) (MCPTokenInfo, error) {
	var info MCPTokenInfo
	err := s.db.QueryRow(`SELECT created_at,last_used FROM account_mcp_tokens WHERE account_id=?`, accountID).
		Scan(&info.CreatedAt, &info.LastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return MCPTokenInfo{Exists: false}, nil
	}
	if err != nil {
		return MCPTokenInfo{}, err
	}
	info.Exists = true
	return info, nil
}

// --- helpers ---

func (s *Store) affect(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	// modernc surfaces "constraint failed: UNIQUE ..." in the error string.
	return err != nil && (strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint failed"))
}
