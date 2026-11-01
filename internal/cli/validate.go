package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/taskmedia/outprint/internal/config"
	"github.com/taskmedia/outprint/internal/packages"
)

type validateOptions struct {
	ServiceConfig string
	TemplatesRoot string
	ConfigsRoot   string
	Template      string
	Config        string
}

// runValidate runs the same loader and checks as service startup and
// prints one finding per line. It exits 1 if any finding is an error;
// warnings alone exit 0.
func runValidate(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := parseValidate(args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "renderer: %v\n%s\n", err, usage)
		return 2
	}
	findings := validateFindings(opts, getenv)
	for _, f := range findings {
		fmt.Fprintln(stdout, f)
	}
	errs, warns := len(packages.Errors(findings)), len(packages.Warnings(findings))
	if errs > 0 {
		fmt.Fprintf(stdout, "validate: failed with %d error(s), %d warning(s)\n", errs, warns)
		return 1
	}
	fmt.Fprintf(stdout, "validate: ok, %d warning(s)\n", warns)
	return 0
}

func parseValidate(args []string, getenv func(string) string) (validateOptions, error) {
	var o validateOptions
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.ServiceConfig, "service-config", "", "service configuration file (default: $"+config.EnvServiceConfig+")")
	fs.StringVar(&o.TemplatesRoot, "templates-root", firstNonEmpty(getenv(envTemplatesRoot), defaultTemplatesRoot), "Templates root")
	fs.StringVar(&o.ConfigsRoot, "configs-root", firstNonEmpty(getenv(envConfigsRoot), defaultConfigsRoot), "Configurations root")
	fs.StringVar(&o.Template, "template", "", "validate only this Template")
	fs.StringVar(&o.Config, "config", "", "validate only this Configuration")
	if err := fs.Parse(args); err != nil {
		return validateOptions{}, err
	}
	if fs.NArg() != 0 {
		return validateOptions{}, errors.New("validate takes no positional arguments")
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	selects := explicit["templates-root"] || explicit["configs-root"] || explicit["template"] || explicit["config"]
	if o.ServiceConfig != "" && selects {
		return validateOptions{}, errors.New("with a service configuration the roots and the allow-list come from it (roots can be overridden by environment); do not combine it with --templates-root, --configs-root, --template or --config")
	}
	if o.ServiceConfig == "" && !selects {
		o.ServiceConfig = getenv(config.EnvServiceConfig)
	}
	return o, nil
}

func validateFindings(o validateOptions, getenv func(string) string) []packages.Finding {
	if o.ServiceConfig == "" {
		_, findings := packages.Inspect(packages.Roots{Templates: o.TemplatesRoot, Configurations: o.ConfigsRoot}, o.Template, o.Config)
		return findings
	}
	svc, errs := config.Load(o.ServiceConfig, getenv)
	if len(errs) > 0 {
		findings := make([]packages.Finding, len(errs))
		for i, err := range errs {
			findings[i] = packages.Finding{Severity: packages.SeverityError, Package: "service", Location: o.ServiceConfig, Message: err.Error()}
		}
		return findings
	}
	_, findings := packages.Load(svc)
	return findings
}
