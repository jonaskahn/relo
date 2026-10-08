// Template vocabulary aliases from the catalog core.
package templates

import (
	"github.com/jonaskahn/relo/internal/catalog"
)

// Kind names what a connection is, reusing the catalog vocabulary so the
// registry and the use cases name one connection the same way.
type (
	Kind = catalog.Kind
	// FormatOption names one wire format a connection template offers.
	FormatOption = catalog.FormatOption
	// Variable names one template placeholder an operator fills in.
	Variable = catalog.Variable
	// LoginKind names how an operator signs in.
	LoginKind = catalog.LoginKind
	// LoginMethod names one login path a template offers.
	LoginMethod = catalog.LoginMethod
	// Template is one connection blueprint the registry serves.
	Template = catalog.Template
)

// Template kinds and login methods reuse the catalog vocabulary, so the
// registry and the use cases name one connection the same way.
const (
	KindSignIn = catalog.KindSignIn
	KindKey    = catalog.KindKey
	KindCloud  = catalog.KindCloud
	KindLocal  = catalog.KindLocal

	LoginBrowser = catalog.LoginBrowser
	LoginDevice  = catalog.LoginDevice
	LoginCLI     = catalog.LoginCLI
)
