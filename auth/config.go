package auth

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds the Cognito settings. All fields are required.
type Config struct {
	BaseURL       string // e.g. https://fretboard.tail9292f9.ts.net (no trailing slash)
	Issuer        string // https://cognito-idp.<region>.amazonaws.com/<poolId>
	ClientID      string
	ClientSecret  string
	CognitoDomain string // https://fretboard-viz-login.auth.us-east-2.amazoncognito.com
}

// ConfigFromEnv reads the configuration from the environment. ok is false
// (and err nil) only when every variable is unset, meaning sign-in is
// disabled. A partial or malformed configuration is an error, never a silent
// fallback to disabled.
func ConfigFromEnv() (cfg Config, ok bool, err error) {
	cfg = Config{
		BaseURL:       strings.TrimSpace(os.Getenv("FRETBOARD_BASE_URL")),
		Issuer:        strings.TrimSpace(os.Getenv("COGNITO_ISSUER")),
		ClientID:      strings.TrimSpace(os.Getenv("COGNITO_CLIENT_ID")),
		ClientSecret:  strings.TrimSpace(os.Getenv("COGNITO_CLIENT_SECRET")),
		CognitoDomain: strings.TrimSpace(os.Getenv("COGNITO_DOMAIN")),
	}
	if cfg == (Config{}) {
		return Config{}, false, nil
	}
	if err := cfg.validate(); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

// validate checks that every field is present and well formed. Messages name
// the environment variable and never include values (ClientSecret is one).
func (c Config) validate() error {
	var missing []string
	for _, f := range []struct{ env, val string }{
		{"FRETBOARD_BASE_URL", c.BaseURL},
		{"COGNITO_ISSUER", c.Issuer},
		{"COGNITO_CLIENT_ID", c.ClientID},
		{"COGNITO_CLIENT_SECRET", c.ClientSecret},
		{"COGNITO_DOMAIN", c.CognitoDomain},
	} {
		if f.val == "" {
			missing = append(missing, f.env)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("auth config incomplete: set %s (or unset every auth variable to disable sign-in)", strings.Join(missing, ", "))
	}
	if err := checkURL(c.BaseURL, "FRETBOARD_BASE_URL", true, false); err != nil {
		return err
	}
	if err := checkURL(c.Issuer, "COGNITO_ISSUER", false, true); err != nil {
		return err
	}
	return checkURL(c.CognitoDomain, "COGNITO_DOMAIN", false, false)
}

// checkURL requires an absolute URL with a host and no credentials, query or
// fragment. The scheme must be https unless allowHTTP (local development).
// Without allowPath, any path (a trailing slash included) is rejected.
func checkURL(s, env string, allowHTTP, allowPath bool) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("%s must be an absolute URL with a host and no credentials, query or fragment", env)
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return fmt.Errorf("%s must use https", env)
	}
	if !allowPath && u.Path != "" {
		return errors.New(env + " must be just scheme and host (no path or trailing slash)")
	}
	return nil
}
