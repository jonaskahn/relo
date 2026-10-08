package catalog

// Kind describes the type of provider a template declares.
type Kind string

const (
	// KindSignIn offers a browser or device sign-in rather than a key.
	KindSignIn Kind = "signin"
	// KindKey takes an API key the operator pastes.
	KindKey Kind = "key"
	// KindCloud takes vendor cloud credentials.
	KindCloud Kind = "cloud"
	// KindLocal points at a runtime on this machine.
	KindLocal Kind = "local"
)

// FormatOption defines one API format option for a provider.
type FormatOption struct {
	Format         APIFormat
	DefaultBaseURL string
	KeyHeader      KeyHeader
	ModelsFormat   ModelsFormat
	Label          string
}

// Variable defines a configuration variable needed in the base URL or request.
type Variable struct {
	Name        string
	Label       string
	Placeholder string
	Required    bool
	Options     []string
}

// LoginKind names how a human completes a login, which the console shows
// so an operator knows what is about to happen before it starts.
type LoginKind string

// OpenCodeFreeTemplate is the signed-out OpenCode Zen connection.
const OpenCodeFreeTemplate = "opencode-free"

const (
	// LoginBrowser opens a page the operator signs in on.
	LoginBrowser LoginKind = "browser"
	// LoginDevice shows a code the operator enters on another device.
	LoginDevice LoginKind = "device"
	// LoginCLI imports a session an installed application already holds.
	LoginCLI LoginKind = "cli"
)

// LoginMethod is one way into a provider account. A template with several
// methods offers each one, and the flow name picks which one runs.
type LoginMethod struct {
	Flow string
	Kind LoginKind
}

// Template represents how to configure and connect a provider.
type Template struct {
	ID               string
	Label            string
	Kind             Kind
	Origin           Origin
	Auth             Auth
	KeyHeader        KeyHeader
	DefaultFormat    APIFormat
	AvailableFormats []FormatOption
	DefaultBaseURL   string
	// RequiresStream reports an upstream that refuses a one-shot request, so
	// the relay has to ask for the stream and reassemble it for a client that
	// did not ask to stream.
	RequiresStream bool
	// RefusesMaxOutputTokens reports an upstream whose dialect has no
	// max-output ceiling parameter, so the codec leaves it out rather than
	// send a request the upstream would refuse.
	RefusesMaxOutputTokens bool
	Variables              []Variable
	// ModelsSource names how a connection fills its roster: "listing" for one
	// whose provider publishes a list, "manual" for one that takes the ids an
	// operator types. Nothing declares anything else; a stored row may still
	// read "modelsdev", which is what an older build wrote for a row it added
	// from the model catalog.
	ModelsSource        string
	ModelsFormat        ModelsFormat
	ModelsDevProviderID string
	DocURL              string
	KeyEnv              []string
	LoginFlows          []string
	// LoginMethods is how an operator signs in, which LoginFlows is
	// derived from. A template with none cannot be signed into.
	LoginMethods      []LoginMethod
	Headers           map[string]string
	UnsupportedReason string
	// ModelsDevModels is how many models the template's models.dev entry
	// lists, which the add list shows as a size hint. It is filled in when
	// the templates are read, because it depends on the saved copy.
	ModelsDevModels int
	// FilterModels narrows the ids a roster keeps when the provider's list
	// carries entries the connection does not serve.
	FilterModels func(string) bool
}

// FormatOption returns the declared option for one format, which is what a
// probe uses when the request names a format and leaves the header and listing
// dialect unstated.
func (t Template) FormatOption(format APIFormat) (FormatOption, bool) {
	for _, option := range t.AvailableFormats {
		if option.Format == format {
			return option, true
		}
	}
	return FormatOption{}, false
}

// Normalize fills the fields derived from others, so a reader never has to
// know which of two spellings a template declares.
func (t *Template) Normalize() {
	if len(t.LoginMethods) == 0 {
		return
	}
	t.LoginFlows = make([]string, 0, len(t.LoginMethods))
	for _, method := range t.LoginMethods {
		t.LoginFlows = append(t.LoginFlows, method.Flow)
	}
}
