// Package settings is the one place that names every environment variable
// Outprint reads. Each setting has a name, the component that reads it, a
// type, a default, a tier and a description. The packages take the names from
// here; the settings page of the Reference is generated from the same list
// (scripts/docs-settings), so the code and the documentation cannot drift.
//
// The package has no imports, so every other package can use it without an
// import cycle. It never reads the environment itself: parsing and defaults
// stay with the packages that use a value.
package settings

// Names of the settings.
const (
	EnvOutlineURL       = "OUTLINE_URL"
	EnvOutlinePublicURL = "OUTLINE_PUBLIC_URL"

	EnvListen         = "OUTPRINT_LISTEN"
	EnvWorkerURL      = "OUTPRINT_WORKER_URL"
	EnvWorkerToken    = "OUTPRINT_WORKER_TOKEN"
	EnvServiceConfig  = "OUTPRINT_SERVICE_CONFIG"
	EnvTemplatesRoot  = "OUTPRINT_TEMPLATES_ROOT"
	EnvConfigsRoot    = "OUTPRINT_CONFIGURATIONS_ROOT"
	EnvTimeout        = "OUTPRINT_TIMEOUT"
	EnvLogLevel       = "OUTPRINT_LOG_LEVEL"
	EnvMaxPages       = "OUTPRINT_MAX_PAGES"
	EnvMaxPDFBytes    = "OUTPRINT_MAX_PDF_BYTES"
	EnvAdmissionSlots = "OUTPRINT_ADMISSION_SLOTS"

	EnvAdmissionQueueDepth = "OUTPRINT_ADMISSION_QUEUE_DEPTH"
	EnvAdmissionPerUser    = "OUTPRINT_ADMISSION_PER_USER"
	EnvAdmissionMaxWait    = "OUTPRINT_ADMISSION_MAX_WAIT"

	EnvButtonPlacement = "OUTPRINT_BUTTON_PLACEMENT"
	EnvButtonLabels    = "OUTPRINT_BUTTON_LABELS"

	EnvInjectorUpstream     = "OUTPRINT_INJECTOR_UPSTREAM"
	EnvInjectorListen       = "OUTPRINT_INJECTOR_LISTEN"
	EnvInjectorGatewayURL   = "OUTPRINT_INJECTOR_GATEWAY_URL"
	EnvInjectorCSPAllowSelf = "OUTPRINT_INJECTOR_CSP_ALLOW_SELF"
	EnvInjectorLogLevel     = "OUTPRINT_INJECTOR_LOG_LEVEL"
)

// Tier says who needs to know about a setting.
type Tier string

const (
	// MustSet settings have no usable default: the service refuses to start
	// without them. The Install guide documents them.
	MustSet Tier = "must set"
	// Tuning settings have a default that works. Only the Reference lists them.
	Tuning Tier = "tuning"
)

// Setting is one environment variable.
type Setting struct {
	Name string
	// Component is the part that reads it; the Reference groups by it.
	Component string
	// Type is how the value is written (integer, duration, URL, and so on),
	// as Markdown with the type in code marks.
	Type string
	// Default is the value in use when the setting is unset, as Markdown
	// (a value in code marks). Empty means there is none.
	Default     string
	Tier        Tier
	Description string
}

// Components, in the order the Reference shows them.
const (
	Gateway          = "Gateway"
	Worker           = "Worker"
	GatewayAndWorker = "Gateway and Worker"
	PDFAction        = "PDF action"
	Injector         = "Injector"
)

// All returns every setting in the order of the Reference. The slice is a
// copy.
func All() []Setting {
	out := make([]Setting, len(registry))
	copy(out, registry)
	return out
}

var registry = []Setting{
	{Name: EnvOutlineURL, Component: Gateway, Type: "`URL`", Tier: MustSet,
		Description: "The base URL of Outline, as the Gateway reaches it. The Gateway does not start without it."},
	{Name: EnvWorkerURL, Component: Gateway, Type: "`URL`", Tier: MustSet,
		Description: "The base URL of the Worker. The Gateway does not start without it. The Compose file sets it."},
	{Name: EnvOutlinePublicURL, Component: Gateway, Type: "`URL`", Tier: Tuning,
		Description: "The URL Outline names itself, when `OUTLINE_URL` is an internal address. The Gateway uses it to recognise the images of a document."},
	{Name: EnvAdmissionSlots, Component: Gateway, Type: "`integer`", Default: "`1`", Tier: Tuning,
		Description: "How many PDFs are made at once. At least 1. Overrides `slots` in the service configuration."},
	{Name: EnvAdmissionQueueDepth, Component: Gateway, Type: "`integer`", Default: "`4`", Tier: Tuning,
		Description: "How many requests wait for a free slot. 0 or more. Overrides `queueDepth` in the service configuration."},
	{Name: EnvAdmissionPerUser, Component: Gateway, Type: "`integer`", Default: "`1`", Tier: Tuning,
		Description: "How many PDFs one user may have queued and running together. At least 1. Overrides `perUser` in the service configuration."},
	{Name: EnvAdmissionMaxWait, Component: Gateway, Type: "`duration`", Default: "`60s`", Tier: Tuning,
		Description: "How long a queued request waits for a slot, for example `30s`. Longer than zero. Overrides `maxWait` in the service configuration."},

	{Name: EnvMaxPages, Component: Worker, Type: "`integer`", Default: "`500`", Tier: Tuning,
		Description: "The most pages a PDF may have. A positive integer. A malformed value stops the Worker."},
	{Name: EnvMaxPDFBytes, Component: Worker, Type: "`integer` (bytes)", Default: "`52428800`", Tier: Tuning,
		Description: "The largest PDF the Worker returns, in bytes: 50 MiB. From 1 up to that value; it can be lowered but not raised."},

	{Name: EnvListen, Component: GatewayAndWorker, Type: "`address`", Default: "`:8080` (Gateway), `:8081` (Worker)", Tier: Tuning,
		Description: "The address to listen on, for example `:9000`."},
	{Name: EnvWorkerToken, Component: GatewayAndWorker, Type: "`string` (a secret)", Tier: Tuning,
		Description: "A shared token. When set on the Worker, it only accepts requests that carry it, so the Gateway needs the same value. Empty means no token."},
	{Name: EnvTimeout, Component: GatewayAndWorker, Type: "`duration`", Default: "`60s`", Tier: Tuning,
		Description: "The time one PDF may take, for example `90s`. A value that is not a positive duration is ignored."},
	{Name: EnvLogLevel, Component: GatewayAndWorker, Type: "one of `debug`, `info`, `warn`, `error`", Default: "`info`", Tier: Tuning,
		Description: "How much is logged. An unknown value is ignored. The command line logs at `warn` unless set."},
	{Name: EnvServiceConfig, Component: GatewayAndWorker, Type: "`path`", Tier: Tuning,
		Description: "The service configuration file. The image sets `/opt/outprint/service.yaml`. Without it, a service needs `--service-config`."},
	{Name: EnvTemplatesRoot, Component: GatewayAndWorker, Type: "`path`", Tier: Tuning,
		Description: "The folder with the Template packages. Wins over `templatesRoot` in the service configuration. The image sets `/opt/outprint/templates`."},
	{Name: EnvConfigsRoot, Component: GatewayAndWorker, Type: "`path`", Tier: Tuning,
		Description: "The folder with the Configuration packages. Wins over `configurationsRoot` in the service configuration. The image sets `/opt/outprint/configurations`."},

	{Name: EnvButtonPlacement, Component: PDFAction, Type: "one of `menu`, `button`", Default: "`menu`", Tier: Tuning,
		Description: "Where the PDF action sits. `menu` puts it in the Export submenu of the document menu, `button` in a header button before Share."},
	{Name: EnvButtonLabels, Component: PDFAction, Type: "`JSON object`", Default: "the built-in texts", Tier: Tuning,
		Description: "Replaces the texts of the action. Keys are `menu`, `button` and `tooltip`, each a map from a language such as `de` to a text of 1 to 80 characters."},

	{Name: EnvInjectorUpstream, Component: Injector, Type: "`URL`", Tier: MustSet,
		Description: "The base URL of Outline. The only place the injector sends requests to, besides the Gateway. The injector does not start without it."},
	{Name: EnvInjectorGatewayURL, Component: Injector, Type: "`URL`", Tier: Tuning,
		Description: "The base URL of the Gateway. When set, requests below the Gateway prefix go there. Empty leaves the routing to an edge proxy."},
	{Name: EnvInjectorListen, Component: Injector, Type: "`address`", Default: "`:8090`", Tier: Tuning,
		Description: "The address the injector listens on."},
	{Name: EnvInjectorCSPAllowSelf, Component: Injector, Type: "`boolean`", Default: "`false`", Tier: Tuning,
		Description: "Adds `'self'` to the script policy of Outline, so the page may load the script from the injector's origin."},
	{Name: EnvInjectorLogLevel, Component: Injector, Type: "one of `debug`, `info`, `warn`, `error`", Default: "`info`", Tier: Tuning,
		Description: "How much the injector logs. An unknown value is ignored."},
}
