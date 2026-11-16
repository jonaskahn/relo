// Package claudebindings maps a native Claude model id to the Relo model or
// route that answers it when Claude Code is wired through proxy mode.
//
// The bindings of one integration are a single JSON document the docstore keeps
// in memory, so resolving a request is a map lookup and a write replaces the
// whole document.
package claudebindings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

// Namespace is where the bindings documents live, one per integration.
const Namespace = "claude.bindings"

// SchemaVersion is the shape the documents are written in.
const SchemaVersion = 1

// How a binding sends a request.
const (
	// DirectionReplace sends the request to Relo instead of the vendor.
	DirectionReplace = "replace"
	// DirectionFallback tries the vendor first and replays the request to Relo
	// when the vendor refuses it before answering.
	DirectionFallback = "fallback"
)

// What a replaced request does when Relo cannot answer it.
const (
	// OnFailureNone returns Relo's refusal, so a bound request never spends the
	// operator's vendor quota unasked.
	OnFailureNone = "none"
	// OnFailureNative replays the request to the vendor.
	OnFailureNative = "native"
)

// What a binding points at.
const (
	TargetRoute = "route"
	TargetModel = "model"
)

const (
	maxBindings = 200
	maxIDLength = 128
)

// Errors a write is refused with.
var (
	ErrInvalidBinding = errors.New("invalid binding")
	ErrInvalidTarget  = errors.New("invalid binding target")
	ErrTooMany        = errors.New("too many bindings")
)

// Target is the Relo side of a binding: a route, or one connection's model. It
// stores identifiers, never a spelled alias, so a rename cannot break it.
type Target struct {
	Kind       string `json:"kind"`
	GroupID    string `json:"group_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	ModelID    string `json:"model_id,omitempty"`
}

// Binding sends one native Claude id to a target.
type Binding struct {
	NativeID  string `json:"native_id"`
	Target    Target `json:"target"`
	Direction string `json:"direction"`
	OnFailure string `json:"on_failure"`
	Enabled   bool   `json:"enabled"`
}

// Document is every binding of one integration, with the index a request is
// resolved through. The index is built when the document is read.
type Document struct {
	Bindings []Binding
	byID     map[string]Binding
}

type wireDocument struct {
	Bindings []Binding `json:"bindings"`
}

// NewDocument returns a document over the bindings, dropping any that are not
// well formed and keeping the first of two that name the same id.
func NewDocument(bindings []Binding) Document {
	document := Document{byID: make(map[string]Binding, len(bindings))}
	for _, binding := range bindings {
		normalized, err := normalize(binding)
		if err != nil {
			continue
		}
		if _, taken := document.byID[normalized.NativeID]; taken {
			continue
		}
		document.byID[normalized.NativeID] = normalized
		document.Bindings = append(document.Bindings, normalized)
	}
	return document
}

// Lookup answers the binding for a request's model, whatever date or
// million-token marker the client spelled it with.
func (d Document) Lookup(model string) (Binding, bool) {
	binding, found := d.byID[NormalizeID(model)]
	return binding, found
}

// Decode reads a stored document. A version this build does not write is
// refused, so a newer build's document is never half-read.
func Decode(version int, body []byte) (Document, error) {
	if version != SchemaVersion {
		return Document{}, fmt.Errorf("claude bindings schema %d is not supported", version)
	}
	var stored wireDocument
	if err := json.Unmarshal(body, &stored); err != nil {
		return Document{}, err
	}
	return NewDocument(stored.Bindings), nil
}

// Encode writes a document in the current shape.
func Encode(document Document) ([]byte, int, error) {
	body, err := json.Marshal(wireDocument{Bindings: document.Bindings})
	return body, SchemaVersion, err
}

// NormalizeID is the key a native id is stored and looked up under: lower
// case, without the million-token marker and without a trailing release date.
func NormalizeID(model string) string {
	bare := catalog.StripContextSuffix(strings.ToLower(strings.TrimSpace(model)))
	return catalog.NativeModelKey(bare)
}

func normalize(binding Binding) (Binding, error) {
	binding.NativeID = NormalizeID(binding.NativeID)
	if err := validID(binding.NativeID); err != nil {
		return Binding{}, err
	}
	if binding.Direction == "" {
		binding.Direction = DirectionReplace
	}
	if binding.OnFailure == "" {
		binding.OnFailure = OnFailureNone
	}
	if binding.Direction != DirectionReplace && binding.Direction != DirectionFallback {
		return Binding{}, fmt.Errorf("%s: %w (direction %q)", binding.NativeID, ErrInvalidBinding, binding.Direction)
	}
	if binding.OnFailure != OnFailureNone && binding.OnFailure != OnFailureNative {
		return Binding{}, fmt.Errorf("%s: %w (on_failure %q)", binding.NativeID, ErrInvalidBinding, binding.OnFailure)
	}
	if err := validTargetShape(binding.Target); err != nil {
		return Binding{}, fmt.Errorf("%s: %w", binding.NativeID, err)
	}
	return binding, nil
}

func validID(id string) error {
	if id == "" || len(id) > maxIDLength || !catalog.NativeClaudeID(id) || !strings.HasPrefix(id, catalog.ClaudePrefix) {
		return fmt.Errorf("%q: %w (a native Claude id starts with claude-)", id, ErrInvalidBinding)
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return fmt.Errorf("%q: %w (unexpected character)", id, ErrInvalidBinding)
		}
	}
	return nil
}

func validTargetShape(target Target) error {
	switch target.Kind {
	case TargetRoute:
		if strings.TrimSpace(target.GroupID) == "" {
			return fmt.Errorf("%w (a route is required)", ErrInvalidTarget)
		}
	case TargetModel:
		if strings.TrimSpace(target.ProviderID) == "" || strings.TrimSpace(target.ModelID) == "" {
			return fmt.Errorf("%w (a connection and a model are required)", ErrInvalidTarget)
		}
	default:
		return fmt.Errorf("%w (kind %q)", ErrInvalidTarget, target.Kind)
	}
	return nil
}
