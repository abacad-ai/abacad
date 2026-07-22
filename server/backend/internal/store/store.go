// Package store is the SQLite persistence layer: accounts, sessions, devices,
// and scoped API keys. Pure-Go driver (modernc.org/sqlite, no CGO) so the server
// is a single static-ish binary that's trivial to deploy on one host.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
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
	apiKeyPrefix      = "abd_key"
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
	Version   string // last version the client reported on connect; "" if never
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

// migrate runs every migrations/*.sql file in lexical order. All statements are
// idempotent (CREATE TABLE IF NOT EXISTS ...), so re-running the whole set on
// each boot is safe — this doubles as the "apply new migrations" path without a
// version table.
//
// The one shape pure SQL can't make idempotent is `ALTER TABLE ... ADD COLUMN`:
// SQLite has no `IF NOT EXISTS` for it, so the second boot re-runs it and errors
// "duplicate column name". That error is benign and can only mean the column is
// already there, so we treat it as a no-op — keeping the re-run-every-boot model
// intact while still allowing additive column migrations.
func (s *Store) migrate() error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(sqlBytes)); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue // ADD COLUMN already applied on an earlier boot
			}
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
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

// --- OAuth identities (external login providers linked to accounts) ---

// AccountByGoogleSub resolves a Google subject id to its linked account.
func (s *Store) AccountByGoogleSub(sub string) (Account, error) {
	var accountID string
	err := s.db.QueryRow(
		`SELECT account_id FROM account_identities WHERE provider='google' AND subject=?`, sub).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	return s.AccountByID(accountID)
}

// LinkGoogleAccount returns the account for a verified Google identity, creating
// or linking one as needed, and reports whether the account was newly created.
// Matching precedence:
//
//  1. an existing google link for this subject — the returning-user path;
//  2. an existing account with the same email — link the Google identity to it
//     (so a password account and its Google sign-in are the same account);
//  3. otherwise a fresh passwordless account (empty password_hash).
//
// The caller must only pass an email Google marked verified — matching an
// unverified email to an existing account would let anyone claim it.
func (s *Store) LinkGoogleAccount(sub, email string) (acc Account, created bool, err error) {
	if acc, err = s.AccountByGoogleSub(sub); err == nil {
		return acc, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Account{}, false, err
	}

	if acc, err = s.AccountByEmail(email); err == nil {
		_, err = s.db.Exec(
			`INSERT OR IGNORE INTO account_identities(provider,subject,account_id,email,created_at)
			 VALUES('google',?,?,?,?)`, sub, acc.ID, email, now())
		return acc, false, err
	} else if !errors.Is(err, ErrNotFound) {
		return Account{}, false, err
	}

	// New passwordless account + identity, atomically.
	acc = Account{ID: auth.NewID("acc"), Email: email, PasswordHash: "", CreatedAt: now()}
	tx, err := s.db.Begin()
	if err != nil {
		return Account{}, false, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO accounts(id,email,password_hash,created_at) VALUES(?,?,?,?)`,
		acc.ID, acc.Email, acc.PasswordHash, acc.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return Account{}, false, ErrEmailTaken
		}
		return Account{}, false, err
	}
	if _, err = tx.Exec(
		`INSERT INTO account_identities(provider,subject,account_id,email,created_at)
		 VALUES('google',?,?,?,?)`, sub, acc.ID, email, now()); err != nil {
		return Account{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Account{}, false, err
	}
	return acc, true, nil
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
// plaintext device token (shown once; only its hash is stored). platform is a
// free-form tag (e.g. "android", "macos"); "" leaves it unset.
func (s *Store) CreateDevice(accountID, name, platform string) (Device, string, error) {
	token := auth.NewSecret(deviceTokenPrefix)
	d := Device{ID: auth.NewDeviceID(), AccountID: accountID, Name: name, Platform: platform, CreatedAt: now()}
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
		`SELECT id,account_id,name,platform,created_at,last_seen,version FROM devices WHERE token_hash=?`, tokenHash))
}

// DeviceByID resolves a device by its (non-secret) id alone. Used by the browser
// device surface, where the id in the request Host is the addressing/auth key —
// unlike a token, the id is not a shared secret in the usual sense, so callers
// that rely on it (only the Host router) must intend exactly that.
func (s *Store) DeviceByID(deviceID string) (Device, error) {
	return s.scanDevice(s.db.QueryRow(
		`SELECT id,account_id,name,platform,created_at,last_seen,version FROM devices WHERE id=?`, deviceID))
}

// DeviceOwnedBy returns a device only if it belongs to accountID.
func (s *Store) DeviceOwnedBy(deviceID, accountID string) (Device, error) {
	return s.scanDevice(s.db.QueryRow(
		`SELECT id,account_id,name,platform,created_at,last_seen,version FROM devices WHERE id=? AND account_id=?`,
		deviceID, accountID))
}

// DevicesByAccount lists an account's devices, most-recently-active first.
func (s *Store) DevicesByAccount(accountID string) ([]Device, error) {
	rows, err := s.db.Query(
		`SELECT id,account_id,name,platform,created_at,last_seen,version FROM devices
		  WHERE account_id=? ORDER BY last_seen DESC, created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeen, &d.Version); err != nil {
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

// SetDeviceVersion records the version a client reported on connect. A blank
// version (older client that doesn't report one) is ignored so it doesn't erase
// a version an earlier connect captured. Best-effort, like TouchDevice.
func (s *Store) SetDeviceVersion(deviceID, version string) {
	if version == "" {
		return
	}
	_, _ = s.db.Exec(`UPDATE devices SET version=? WHERE id=?`, version, deviceID)
}

func (s *Store) scanDevice(row *sql.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.AccountID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeen, &d.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

// --- Device pairing (RFC 8628 device-authorization grant) ---

// Pairing status values.
const (
	PairingPending  = "pending"
	PairingApproved = "approved"
	PairingDenied   = "denied"
)

const pairingTokenPrefix = "abd_pair"

// Pairing is one in-flight `abacad connect` handshake: a secret device_code held
// by the CLI, a short user_code typed by the human, and (once approved) the
// account + device details to mint. Never holds a device token.
type Pairing struct {
	DeviceCode string
	UserCode   string
	Status     string
	AccountID  string // "" until approved
	Name       string
	Platform   string
	CreatedAt  int64
	ExpiresAt  int64
	Consumed   bool
}

// CreatePairing opens a pending pairing valid for ttl and returns the secret
// device_code (CLI-held) and the short human-typed user_code. platform is the
// CLI's self-reported OS (may be ""), stored so the approval page can show what
// it's about to authorize. The user_code is UNIQUE; on the astronomically rare
// clash it regenerates and retries.
func (s *Store) CreatePairing(platform string, ttl time.Duration) (deviceCode, userCode string, err error) {
	deviceCode = auth.NewSecret(pairingTokenPrefix)
	created := now()
	expires := time.Now().Add(ttl).Unix()
	for attempt := 0; attempt < 5; attempt++ {
		userCode = auth.NewUserCode()
		_, err = s.db.Exec(
			`INSERT INTO device_pairings(device_code,user_code,status,platform,created_at,expires_at)
			 VALUES(?,?,?,?,?,?)`, deviceCode, userCode, PairingPending, platform, created, expires)
		if err == nil {
			return deviceCode, userCode, nil
		}
	}
	return "", "", err
}

// PairingByDeviceCode returns a pairing by its secret device_code, with no expiry
// filter — the poller must distinguish "expired" from "unknown" itself.
func (s *Store) PairingByDeviceCode(deviceCode string) (Pairing, error) {
	return s.scanPairing(s.db.QueryRow(
		`SELECT device_code,user_code,status,COALESCE(account_id,''),name,platform,created_at,expires_at,consumed
		   FROM device_pairings WHERE device_code=?`, deviceCode))
}

// PairingByUserCode returns a non-expired pairing by its user_code — used by the
// approval page to show what's being authorized before the human commits.
func (s *Store) PairingByUserCode(userCode string) (Pairing, error) {
	return s.scanPairing(s.db.QueryRow(
		`SELECT device_code,user_code,status,COALESCE(account_id,''),name,platform,created_at,expires_at,consumed
		   FROM device_pairings WHERE user_code=? AND expires_at > ?`, userCode, now()))
}

// ApprovePairing binds a still-pending, unexpired pairing to the approving
// account with the chosen name. platform is an optional override: an empty value
// preserves the platform the CLI reported at start (the common case — the box
// knows its own OS), while a non-empty value lets the approver correct it.
// Returns ErrNotFound if the code is unknown, expired, or already resolved.
func (s *Store) ApprovePairing(userCode, accountID, name, platform string) error {
	return s.affect(s.db.Exec(
		`UPDATE device_pairings
		    SET status=?, account_id=?, name=?,
		        platform = CASE WHEN ? <> '' THEN ? ELSE platform END
		  WHERE user_code=? AND status=? AND expires_at > ?`,
		PairingApproved, accountID, name, platform, platform, userCode, PairingPending, now()))
}

// ConsumePairing completes an approved pairing: it atomically claims the row
// (approved -> consumed) so a double-poll can't mint two devices, then mints the
// device via the normal CreateDevice path and returns its one-time token. Any
// non-approved / already-consumed / expired state yields ErrNotFound.
func (s *Store) ConsumePairing(deviceCode string) (Device, string, error) {
	if err := s.affect(s.db.Exec(
		`UPDATE device_pairings SET consumed=1
		   WHERE device_code=? AND status=? AND consumed=0 AND expires_at > ?`,
		deviceCode, PairingApproved, now())); err != nil {
		return Device{}, "", err // ErrNotFound: not approved, already consumed, or expired
	}
	p, err := s.PairingByDeviceCode(deviceCode)
	if err != nil {
		return Device{}, "", err
	}
	return s.CreateDevice(p.AccountID, p.Name, p.Platform)
}

func (s *Store) scanPairing(row *sql.Row) (Pairing, error) {
	var p Pairing
	var consumed int
	err := row.Scan(&p.DeviceCode, &p.UserCode, &p.Status, &p.AccountID,
		&p.Name, &p.Platform, &p.CreatedAt, &p.ExpiresAt, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return Pairing{}, ErrNotFound
	}
	p.Consumed = consumed == 1
	return p, err
}

// --- API keys (scoped bearer credentials for /mcp and /connect) ---

// KeyScope is an API key's capability envelope: which devices it may reach, which
// methods it may call, and whether it may open a tunnel. AllDevices / AllMethods
// are wildcards that also cover devices/methods added later — so they take
// precedence over the explicit DeviceIDs / Methods lists (which are consulted
// only when the corresponding wildcard is false).
type KeyScope struct {
	AllDevices  bool
	DeviceIDs   []string
	AllMethods  bool
	Methods     []string
	AllowTunnel bool
}

// AllowsDevice reports whether a key with this scope may drive deviceID.
func (s KeyScope) AllowsDevice(id string) bool {
	if s.AllDevices {
		return true
	}
	for _, d := range s.DeviceIDs {
		if d == id {
			return true
		}
	}
	return false
}

// AllowsMethod reports whether a key with this scope may call an MCP method.
func (s KeyScope) AllowsMethod(name string) bool {
	if s.AllMethods {
		return true
	}
	for _, m := range s.Methods {
		if m == name {
			return true
		}
	}
	return false
}

// AllowsTunnel reports whether a key with this scope may open a /connect tunnel.
func (s KeyScope) AllowsTunnel() bool { return s.AllowTunnel }

// APIKey is one scoped bearer credential. The secret is never stored (only its
// hash); reads expose the scope but never the secret.
type APIKey struct {
	ID        string
	AccountID string
	Name      string
	Scope     KeyScope
	CreatedAt int64
	LastUsed  int64
}

// CreateAPIKey issues a scoped key and returns the plaintext secret (shown once)
// alongside the stored row. Only its hash is persisted. Device ids in the scope
// are written to the join table; the caller is responsible for having verified
// they belong to accountID.
func (s *Store) CreateAPIKey(accountID, name string, scope KeyScope) (string, APIKey, error) {
	token := auth.NewSecret(apiKeyPrefix)
	k := APIKey{ID: auth.NewID("apikey"), AccountID: accountID, Name: name, Scope: scope, CreatedAt: now()}
	tx, err := s.db.Begin()
	if err != nil {
		return "", APIKey{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		`INSERT INTO api_keys(id,account_id,name,token_hash,all_devices,methods,allow_tunnel,created_at,last_used)
		 VALUES(?,?,?,?,?,?,?,?,0)`,
		k.ID, accountID, name, auth.HashToken(token),
		boolToInt(scope.AllDevices), encodeMethods(scope), boolToInt(scope.AllowTunnel), k.CreatedAt); err != nil {
		return "", APIKey{}, err
	}
	if err = insertKeyDevices(tx, k.ID, scope); err != nil {
		return "", APIKey{}, err
	}
	if err = tx.Commit(); err != nil {
		return "", APIKey{}, err
	}
	return token, k, nil
}

// APIKeyScopeByTokenHash resolves an API-key bearer token (already hashed) to its
// account and scope, recording last_used. This is the /mcp and /connect auth entry
// point.
func (s *Store) APIKeyScopeByTokenHash(tokenHash string) (accountID string, scope KeyScope, err error) {
	var (
		id          string
		allDevices  int
		methods     string
		allowTunnel int
	)
	err = s.db.QueryRow(
		`SELECT id,account_id,all_devices,methods,allow_tunnel FROM api_keys WHERE token_hash=?`, tokenHash).
		Scan(&id, &accountID, &allDevices, &methods, &allowTunnel)
	if errors.Is(err, sql.ErrNoRows) {
		return "", KeyScope{}, ErrNotFound
	}
	if err != nil {
		return "", KeyScope{}, err
	}
	scope.AllDevices = allDevices != 0
	scope.AllMethods, scope.Methods = decodeMethods(methods)
	scope.AllowTunnel = allowTunnel != 0
	if !scope.AllDevices {
		if scope.DeviceIDs, err = s.apiKeyDeviceIDs(id); err != nil {
			return "", KeyScope{}, err
		}
	}
	_, _ = s.db.Exec(`UPDATE api_keys SET last_used=? WHERE token_hash=?`, now(), tokenHash)
	return accountID, scope, nil
}

// APIKeysByAccount lists an account's keys, newest first (never the secret).
func (s *Store) APIKeysByAccount(accountID string) ([]APIKey, error) {
	rows, err := s.db.Query(
		`SELECT id,account_id,name,all_devices,methods,allow_tunnel,created_at,last_used
		   FROM api_keys WHERE account_id=? ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	var out []APIKey
	for rows.Next() {
		var (
			k           APIKey
			allDevices  int
			methods     string
			allowTunnel int
		)
		if err := rows.Scan(&k.ID, &k.AccountID, &k.Name, &allDevices, &methods, &allowTunnel, &k.CreatedAt, &k.LastUsed); err != nil {
			rows.Close()
			return nil, err
		}
		k.Scope.AllDevices = allDevices != 0
		k.Scope.AllMethods, k.Scope.Methods = decodeMethods(methods)
		k.Scope.AllowTunnel = allowTunnel != 0
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	// Close before the per-key device lookups: with a single writer connection an
	// open rows cursor would block the follow-up queries.
	rows.Close()
	for i := range out {
		if !out[i].Scope.AllDevices {
			ids, err := s.apiKeyDeviceIDs(out[i].ID)
			if err != nil {
				return nil, err
			}
			out[i].Scope.DeviceIDs = ids
		}
	}
	return out, nil
}

// UpdateAPIKey re-configures a key's name and scope (never its secret).
func (s *Store) UpdateAPIKey(id, accountID, name string, scope KeyScope) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := affectTx(tx.Exec(
		`UPDATE api_keys SET name=?, all_devices=?, methods=?, allow_tunnel=? WHERE id=? AND account_id=?`,
		name, boolToInt(scope.AllDevices), encodeMethods(scope), boolToInt(scope.AllowTunnel), id, accountID)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM api_key_devices WHERE api_key_id=?`, id); err != nil {
		return err
	}
	if err := insertKeyDevices(tx, id, scope); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAPIKey removes one of an account's keys (join rows cascade).
func (s *Store) DeleteAPIKey(id, accountID string) error {
	return s.affect(s.db.Exec(`DELETE FROM api_keys WHERE id=? AND account_id=?`, id, accountID))
}

func (s *Store) apiKeyDeviceIDs(keyID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT device_id FROM api_key_devices WHERE api_key_id=?`, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// insertKeyDevices writes a key's explicit device allowlist (a no-op for an
// all-devices scope). INSERT OR IGNORE tolerates duplicate ids in the input.
func insertKeyDevices(tx *sql.Tx, keyID string, scope KeyScope) error {
	if scope.AllDevices {
		return nil
	}
	for _, id := range scope.DeviceIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO api_key_devices(api_key_id,device_id) VALUES(?,?)`, keyID, id); err != nil {
			return err
		}
	}
	return nil
}

// encodeMethods renders a scope's method allowlist for the api_keys.methods
// column: "*" for the all-methods wildcard, else a comma-separated verb list
// (possibly empty for a tunnel-only key).
func encodeMethods(scope KeyScope) string {
	if scope.AllMethods {
		return "*"
	}
	return strings.Join(scope.Methods, ",")
}

func decodeMethods(v string) (all bool, methods []string) {
	if v == "*" {
		return true, nil
	}
	for _, m := range strings.Split(v, ",") {
		if m = strings.TrimSpace(m); m != "" {
			methods = append(methods, m)
		}
	}
	return false, methods
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- Blobs ---

// Blob is the metadata for one stored data-plane object. The bytes live on disk
// under the blob dir, keyed by ID; this row is the addressable, account-scoped
// handle to them.
type Blob struct {
	ID          string
	AccountID   string
	ContentType string
	Size        int64
	SHA256      string // hex
	CreatedAt   int64
}

// CreateBlob records a blob's metadata. The caller has already written the bytes
// to disk; ID must be unique (use auth.NewID("blob")).
func (s *Store) CreateBlob(b Blob) error {
	_, err := s.db.Exec(
		`INSERT INTO blobs(id,account_id,content_type,size,sha256,created_at) VALUES(?,?,?,?,?,?)`,
		b.ID, b.AccountID, b.ContentType, b.Size, b.SHA256, b.CreatedAt)
	return err
}

// BlobByID returns a blob's metadata. Ownership is the caller's to enforce
// (compare AccountID) — this does not scope by account so a 404-vs-403 decision
// stays with the handler.
func (s *Store) BlobByID(id string) (Blob, error) {
	var b Blob
	err := s.db.QueryRow(
		`SELECT id,account_id,content_type,size,sha256,created_at FROM blobs WHERE id=?`, id).
		Scan(&b.ID, &b.AccountID, &b.ContentType, &b.Size, &b.SHA256, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Blob{}, ErrNotFound
	}
	return b, err
}

// --- SSH keys (authorize the jump server) ---

// ErrKeyExists is returned when adding an SSH key whose fingerprint is already
// registered (to any account).
var ErrKeyExists = errors.New("ssh key already registered")

// SSHKey is one authorized public key. The public key is not a secret; it is
// stored in full (normalized authorized_keys line) and indexed by fingerprint.
type SSHKey struct {
	ID          string
	AccountID   string
	Name        string
	Fingerprint string
	PublicKey   string
	CreatedAt   int64
	LastUsed    int64
}

// AddSSHKey registers a public key for an account. fingerprint is the caller's
// precomputed ssh.FingerprintSHA256; publicKey is the normalized authorized_keys
// line. Returns ErrKeyExists if the fingerprint is already known.
func (s *Store) AddSSHKey(accountID, name, fingerprint, publicKey string) (SSHKey, error) {
	k := SSHKey{
		ID: auth.NewID("sshk"), AccountID: accountID, Name: name,
		Fingerprint: fingerprint, PublicKey: publicKey, CreatedAt: now(),
	}
	_, err := s.db.Exec(
		`INSERT INTO ssh_keys(id,account_id,name,fingerprint,public_key,created_at,last_used)
		 VALUES(?,?,?,?,?,?,0)`,
		k.ID, k.AccountID, k.Name, k.Fingerprint, k.PublicKey, k.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return SSHKey{}, ErrKeyExists
		}
		return SSHKey{}, err
	}
	return k, nil
}

// AccountBySSHKeyFingerprint resolves a public-key fingerprint to its owning
// account and records last_used. This is the jump server's authorization lookup.
func (s *Store) AccountBySSHKeyFingerprint(fingerprint string) (Account, error) {
	var accountID string
	err := s.db.QueryRow(`SELECT account_id FROM ssh_keys WHERE fingerprint=?`, fingerprint).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	_, _ = s.db.Exec(`UPDATE ssh_keys SET last_used=? WHERE fingerprint=?`, now(), fingerprint)
	return s.AccountByID(accountID)
}

// SSHKeysByAccount lists an account's registered public keys, newest first.
func (s *Store) SSHKeysByAccount(accountID string) ([]SSHKey, error) {
	rows, err := s.db.Query(
		`SELECT id,account_id,name,fingerprint,public_key,created_at,last_used
		   FROM ssh_keys WHERE account_id=? ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SSHKey
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.AccountID, &k.Name, &k.Fingerprint, &k.PublicKey, &k.CreatedAt, &k.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteSSHKey removes one of an account's keys.
func (s *Store) DeleteSSHKey(id, accountID string) error {
	return s.affect(s.db.Exec(`DELETE FROM ssh_keys WHERE id=? AND account_id=?`, id, accountID))
}

// --- helpers ---
func (s *Store) affect(res sql.Result, err error) error { return affectTx(res, err) }

// affectTx maps a write that touched no rows to ErrNotFound. Standalone (not a
// method) so it works with both *sql.DB and *sql.Tx results.
func affectTx(res sql.Result, err error) error {
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
