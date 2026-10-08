// Package access owns the client-key lifecycle: issuing a key to a coding
// client, rotating and retiring it, and answering what a presented secret is
// checked against.
package access

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	keyaccess "github.com/jonaskahn/relo/internal/access"
	"github.com/jonaskahn/relo/internal/clock"
)

const (
	// Agent names a key issued to one coding client.
	Agent = "agent"
	// Shared names a key an operator deliberately reuses across clients.
	Shared  = "shared"
	nameMax = 64
)

var (
	// ErrNotFound reports an identifier or name no key matches.
	ErrNotFound = keyaccess.ErrKeyNotFound
	// ErrInvalid reports a key request the domain cannot accept.
	ErrInvalid = errors.New("invalid access key")
	// ErrNameTaken reports an active key that already has a name.
	ErrNameTaken = keyaccess.ErrKeyNameTaken
	// ErrOwned reports a key an integration minted, which only that
	// integration may rotate or retire: the key and the wiring it belongs to
	// are one thing, and changing one without the other leaves a machine that
	// cannot reach Relo.
	ErrOwned = errors.New("this key belongs to an integration")
)

// KeyStore persists client keys.
type KeyStore interface {
	List(ctx context.Context) ([]keyaccess.Key, error)
	Get(ctx context.Context, id string) (keyaccess.Key, bool, error)
	Insert(ctx context.Context, key keyaccess.Key) error
	Update(ctx context.Context, update keyaccess.KeyUpdate) error
	Rotate(ctx context.Context, rotation keyaccess.KeyRotation) error
	Revoke(ctx context.Context, id string, nowMs int64) error
	Touch(ctx context.Context, id string, nowMs int64) error
	DeleteExpired(ctx context.Context, nowMs int64, ids []string) (int, error)
}

// AccessKey is one client key as every surface reports it. The secret is
// never part of this shape: only the hint that identifies the key and the
// metadata an operator acts on.
type AccessKey struct {
	ID           string
	Name         string
	Kind         string
	Client       string
	Hint         string
	Generation   int
	Status       string
	ExpiresAtMs  int64
	CreatedAtMs  int64
	UpdatedAtMs  int64
	LastUsedAtMs int64
	// Owner names the integration this key belongs to, and is empty on a key
	// an operator minted for themselves. An owned key is read-only here: the
	// integration page is where it is rotated or retired.
	Owner string
}

// IssuedAccessKey is a key plus the one value that is never stored or read
// back: the token an operator has to keep.
type IssuedAccessKey struct {
	AccessKey
	Token string
}

// NewAccessKey is one key a caller wants issued.
type NewAccessKey struct {
	Name        string
	Kind        string
	Client      string
	ExpiresAtMs int64
	// Owner marks the key as belonging to one integration, which is what makes
	// it read-only on the client keys page.
	Owner string
}

// AccessKeyUpdate is one metadata change to a stored key.
type AccessKeyUpdate struct {
	Name        string
	ExpiresAtMs int64
}

// AccessKeyRecord is the verification view of one stored key: what a
// presented token is checked against, and never the token itself.
type AccessKeyRecord struct {
	ID          string
	Name        string
	Kind        string
	Client      string
	Owner       string
	Digest      string
	ExpiresAtMs int64
	RevokedAtMs int64
}

// Keys is the client-key lifecycle over one store.
type Keys struct {
	store KeyStore
	clock clock.Clock
}

// NewKeys returns the client-key use cases over the given store.
func NewKeys(store KeyStore, clk clock.Clock) *Keys {
	if clk == nil {
		clk = clock.New()
	}
	return &Keys{store: store, clock: clk}
}

// AccessKeys returns every key, newest first, retired ones included so a
// log row that names one stays explainable.
func (k *Keys) AccessKeys(ctx context.Context) ([]AccessKey, error) {
	rows, err := k.store.List(ctx)
	if err != nil {
		return nil, err
	}
	now := k.clock.Now()
	keys := make([]AccessKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, describeAccessKey(row, now))
	}
	return keys, nil
}

// AccessKey returns one key, named by its identifier or by its exact name.
func (k *Keys) AccessKey(ctx context.Context, idOrName string) (AccessKey, error) {
	row, err := k.resolve(ctx, idOrName)
	if err != nil {
		return AccessKey{}, err
	}
	return describeAccessKey(row, k.clock.Now()), nil
}

// CreateAccessKey issues one key and returns the token that goes with it.
// The token is the only time the secret exists outside the operator's hands.
func (k *Keys) CreateAccessKey(ctx context.Context, request NewAccessKey) (IssuedAccessKey, error) {
	name, kind, client, expiresAt, err := validate(request, k.clock.Now())
	if err != nil {
		return IssuedAccessKey{}, err
	}
	if err := k.checkName(ctx, name, ""); err != nil {
		return IssuedAccessKey{}, err
	}
	id, err := keyaccess.NewID()
	if err != nil {
		return IssuedAccessKey{}, err
	}
	token, err := keyaccess.Mint(id)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	now := k.clock.Now().UnixMilli()
	key := keyaccess.Key{
		ID: id, Name: name, Kind: kind, Client: client, TokenDigest: token.Digest,
		TokenHint: token.Hint, Generation: 1, ExpiresAtMs: expiresAt,
		Owner:       strings.TrimSpace(request.Owner),
		CreatedAtMs: now, UpdatedAtMs: now,
	}
	if err := k.store.Insert(ctx, key); err != nil {
		return IssuedAccessKey{}, err
	}
	return IssuedAccessKey{AccessKey: describeAccessKey(key, k.clock.Now()), Token: token.Raw}, nil
}

// UpdateAccessKey changes the name and expiry of one live key.
func (k *Keys) UpdateAccessKey(ctx context.Context, id string, update AccessKeyUpdate) (AccessKey, error) {
	key, err := k.resolve(ctx, id)
	if err != nil {
		return AccessKey{}, err
	}
	if err := refuseOwned(key); err != nil {
		return AccessKey{}, err
	}
	if key.RevokedAtMs > 0 {
		return AccessKey{}, fmt.Errorf("%s: %w (the key is revoked)", key.ID, ErrInvalid)
	}
	name := strings.TrimSpace(update.Name)
	if err := checkName(name); err != nil {
		return AccessKey{}, err
	}
	if err := k.checkName(ctx, name, key.ID); err != nil {
		return AccessKey{}, err
	}
	expiresAt, err := checkExpiry(update.ExpiresAtMs, k.clock.Now())
	if err != nil {
		return AccessKey{}, err
	}
	if err := k.store.Update(ctx, keyaccess.KeyUpdate{ID: key.ID, Name: name, ExpiresAtMs: expiresAt, AtMs: k.clock.Now().UnixMilli()}); err != nil {
		return AccessKey{}, err
	}
	return k.AccessKey(ctx, key.ID)
}

// RotateAccessKey replaces one key's secret, keeping its identity and
// retiring the previous secret in the same write.
func (k *Keys) RotateAccessKey(ctx context.Context, id string) (IssuedAccessKey, error) {
	key, err := k.resolve(ctx, id)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	if err := refuseOwned(key); err != nil {
		return IssuedAccessKey{}, err
	}
	if key.RevokedAtMs > 0 {
		return IssuedAccessKey{}, fmt.Errorf("%s: %w (the key is revoked)", key.ID, ErrInvalid)
	}
	token, err := keyaccess.Mint(key.ID)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	if err := k.store.Rotate(ctx, keyaccess.KeyRotation{ID: key.ID, Digest: token.Digest, Hint: token.Hint, ExpiresAtMs: key.ExpiresAtMs, AtMs: k.clock.Now().UnixMilli()}); err != nil {
		return IssuedAccessKey{}, err
	}
	rotated, err := k.AccessKey(ctx, key.ID)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	return IssuedAccessKey{AccessKey: rotated, Token: token.Raw}, nil
}

// RevokeAccessKey retires one key permanently. Its usage history stays
// attributable, so revocation is a timestamp rather than a delete.
func (k *Keys) RevokeAccessKey(ctx context.Context, id string) error {
	key, err := k.resolve(ctx, id)
	if err != nil {
		return err
	}
	if err := refuseOwned(key); err != nil {
		return err
	}
	if err := k.store.Revoke(ctx, key.ID, k.clock.Now().UnixMilli()); err != nil {
		return err
	}
	return nil
}

// LookupAccessKey returns the verification view of one key, which is what
// the data-plane gate compares a presented secret against.
func (k *Keys) LookupAccessKey(ctx context.Context, id string) (AccessKeyRecord, bool, error) {
	key, found, err := k.store.Get(ctx, strings.TrimSpace(id))
	if err != nil || !found {
		return AccessKeyRecord{}, false, err
	}
	return toRecord(key), true, nil
}

// MarkAccessKeyUsed records that a key authenticated a request.
func (k *Keys) MarkAccessKeyUsed(ctx context.Context, id string) error {
	return k.store.Touch(ctx, id, k.clock.Now().UnixMilli())
}

// OwnedKey returns the live key one owner holds, which is the key an
// integration minted for itself.
func (k *Keys) OwnedKey(ctx context.Context, owner string) (AccessKey, bool, error) {
	keys, err := k.store.List(ctx)
	if err != nil {
		return AccessKey{}, false, err
	}
	for _, key := range keys {
		if key.Owner == owner && key.RevokedAtMs == 0 {
			return describeAccessKey(key, k.clock.Now()), true, nil
		}
	}
	return AccessKey{}, false, nil
}

// RotateOwnedKey replaces the secret of the key one owner holds. This is the
// one path that may rotate an owned key: the integration that owns it
// rewrites its own files with the new secret in the same action.
func (k *Keys) RotateOwnedKey(ctx context.Context, owner string) (IssuedAccessKey, error) {
	owned, found, err := k.OwnedKey(ctx, owner)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	if !found {
		return IssuedAccessKey{}, fmt.Errorf("%s: %w", owner, ErrNotFound)
	}
	key, err := k.resolve(ctx, owned.ID)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	token, err := keyaccess.Mint(key.ID)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	if err := k.store.Rotate(ctx, keyaccess.KeyRotation{ID: key.ID, Digest: token.Digest, Hint: token.Hint, ExpiresAtMs: key.ExpiresAtMs, AtMs: k.clock.Now().UnixMilli()}); err != nil {
		return IssuedAccessKey{}, err
	}
	rotated, err := k.AccessKey(ctx, key.ID)
	if err != nil {
		return IssuedAccessKey{}, err
	}
	return IssuedAccessKey{AccessKey: rotated, Token: token.Raw}, nil
}

// RevokeOwnedKey retires the key one owner holds, which is what turning an
// integration off requires.
func (k *Keys) RevokeOwnedKey(ctx context.Context, owner string) error {
	owned, found, err := k.OwnedKey(ctx, owner)
	if err != nil || !found {
		return err
	}
	return k.store.Revoke(ctx, owned.ID, k.clock.Now().UnixMilli())
}

// DeleteExpiredKeys removes expired operator keys. A revoked key, a live
// key, and a key an integration owns stay. An empty id list removes every
// expired operator key; a named list refuses any id that is not expired.
func (k *Keys) DeleteExpiredKeys(ctx context.Context, ids []string) ([]string, error) {
	now := k.clock.Now()
	targets, err := k.expiredDeleteTargets(ctx, ids, now)
	if err != nil || len(targets) == 0 {
		return targets, err
	}
	if _, err := k.store.DeleteExpired(ctx, now.UnixMilli(), targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func (k *Keys) expiredDeleteTargets(ctx context.Context, ids []string, now time.Time) ([]string, error) {
	if len(ids) == 0 {
		return k.everyExpiredOperator(ctx, now)
	}
	targets := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		key, err := k.resolve(ctx, raw)
		if err != nil {
			return nil, err
		}
		if err := refuseExpiredDelete(key, now); err != nil {
			return nil, err
		}
		if _, found := seen[key.ID]; found {
			continue
		}
		seen[key.ID] = struct{}{}
		targets = append(targets, key.ID)
	}
	return targets, nil
}

func (k *Keys) everyExpiredOperator(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := k.store.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0)
	for _, key := range rows {
		if expiredOperator(key, now) {
			targets = append(targets, key.ID)
		}
	}
	return targets, nil
}

func (k *Keys) resolve(ctx context.Context, idOrName string) (keyaccess.Key, error) {
	raw := strings.TrimSpace(idOrName)
	if raw == "" {
		return keyaccess.Key{}, fmt.Errorf("access key: %w", ErrNotFound)
	}
	if key, found, err := k.store.Get(ctx, raw); err != nil {
		return keyaccess.Key{}, err
	} else if found {
		return key, nil
	}
	keys, err := k.store.List(ctx)
	if err != nil {
		return keyaccess.Key{}, err
	}
	for _, key := range keys {
		if strings.EqualFold(key.Name, raw) {
			return key, nil
		}
	}
	return keyaccess.Key{}, fmt.Errorf("%s: %w", raw, ErrNotFound)
}

func (k *Keys) checkName(ctx context.Context, name, exceptID string) error {
	keys, err := k.store.List(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if key.RevokedAtMs > 0 || key.ID == exceptID {
			continue
		}
		if strings.EqualFold(key.Name, name) {
			return fmt.Errorf("%s: %w", name, ErrNameTaken)
		}
	}
	return nil
}

func validate(request NewAccessKey, now time.Time) (string, string, string, int64, error) {
	name := strings.TrimSpace(request.Name)
	if err := checkName(name); err != nil {
		return "", "", "", 0, err
	}
	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		kind = Agent
	}
	client := strings.TrimSpace(request.Client)
	switch kind {
	case Agent:
		if err := checkClient(client); err != nil {
			return "", "", "", 0, err
		}
	case Shared:
		if client != "" {
			return "", "", "", 0, fmt.Errorf("access_key.client: %w (a shared key names no client)", ErrInvalid)
		}
	default:
		return "", "", "", 0, fmt.Errorf("access_key.kind: %w (must be agent or shared)", ErrInvalid)
	}
	expiresAt, err := checkExpiry(request.ExpiresAtMs, now)
	if err != nil {
		return "", "", "", 0, err
	}
	return name, kind, client, expiresAt, nil
}

func checkName(name string) error {
	if name == "" {
		return fmt.Errorf("access_key.name: %w (a name is required)", ErrInvalid)
	}
	if len(name) > nameMax {
		return fmt.Errorf("access_key.name: %w (at most %d characters)", ErrInvalid, nameMax)
	}
	return nil
}

func checkClient(client string) error {
	if client == "" {
		return fmt.Errorf("access_key.client: %w (an agent key names its client)", ErrInvalid)
	}
	if client == keyaccess.CustomClient {
		return nil
	}
	if _, known := keyaccess.ClientProfileFor(client); !known {
		return fmt.Errorf("access_key.client: %s: %w (known clients: %s, %s)",
			client, ErrInvalid, keyaccess.KnownClientIDs(), keyaccess.CustomClient)
	}
	return nil
}

func checkExpiry(expiresAtMs int64, now time.Time) (int64, error) {
	if expiresAtMs <= 0 {
		return 0, nil
	}
	if !now.Before(time.UnixMilli(expiresAtMs)) {
		return 0, fmt.Errorf("access_key.expires_at: %w (the expiry has already passed)", ErrInvalid)
	}
	return expiresAtMs, nil
}

func describeAccessKey(key keyaccess.Key, now time.Time) AccessKey {
	return AccessKey{
		ID: key.ID, Name: key.Name, Kind: key.Kind, Client: key.Client, Owner: key.Owner,
		Hint: key.TokenHint, Generation: key.Generation, Status: keyaccess.StatusOf(key, now),
		ExpiresAtMs: key.ExpiresAtMs, CreatedAtMs: key.CreatedAtMs,
		UpdatedAtMs: key.UpdatedAtMs, LastUsedAtMs: key.LastUsedAtMs,
	}
}

func toRecord(key keyaccess.Key) AccessKeyRecord {
	return AccessKeyRecord{
		ID: key.ID, Name: key.Name, Kind: key.Kind, Client: key.Client,
		Owner: key.Owner, Digest: key.TokenDigest,
		ExpiresAtMs: key.ExpiresAtMs, RevokedAtMs: key.RevokedAtMs,
	}
}

func refuseOwned(key keyaccess.Key) error {
	if key.Owner == "" {
		return nil
	}
	return fmt.Errorf("%s: %w (%s)", key.Name, ErrOwned, key.Owner)
}

func refuseExpiredDelete(key keyaccess.Key, now time.Time) error {
	if err := refuseOwned(key); err != nil {
		return err
	}
	if key.RevokedAtMs > 0 {
		return fmt.Errorf("%s: %w (the key is revoked)", key.ID, ErrInvalid)
	}
	if keyaccess.StatusOf(key, now) != keyaccess.StatusExpired {
		return fmt.Errorf("%s: %w (the key is not expired)", key.ID, ErrInvalid)
	}
	return nil
}

func expiredOperator(key keyaccess.Key, now time.Time) bool {
	return key.Owner == "" && key.RevokedAtMs == 0 && keyaccess.StatusOf(key, now) == keyaccess.StatusExpired
}
