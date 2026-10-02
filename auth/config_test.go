package auth

import (
	"strings"
	"testing"
)

var envNames = []string{"FRETBOARD_BASE_URL", "COGNITO_ISSUER", "COGNITO_CLIENT_ID", "COGNITO_CLIENT_SECRET", "COGNITO_DOMAIN"}

func fullEnv() map[string]string {
	return map[string]string{
		"FRETBOARD_BASE_URL":    "https://fretboard.tail9292f9.ts.net",
		"COGNITO_ISSUER":        "https://cognito-idp.us-east-2.amazonaws.com/us-east-2_8UtmIgtxM",
		"COGNITO_CLIENT_ID":     "client-id",
		"COGNITO_CLIENT_SECRET": "s3cr3t-value",
		"COGNITO_DOMAIN":        "https://fretboard-viz-login.auth.us-east-2.amazoncognito.com",
	}
}

// setEnv sets exactly the given variables and blanks the rest.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, name := range envNames {
		t.Setenv(name, env[name])
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Run("all unset disables auth", func(t *testing.T) {
		setEnv(t, nil)
		cfg, ok, err := ConfigFromEnv()
		if ok || err != nil || cfg != (Config{}) {
			t.Errorf("got (%+v, %v, %v), want zero, false, nil", cfg, ok, err)
		}
	})
	t.Run("blank values count as unset", func(t *testing.T) {
		env := map[string]string{}
		for _, name := range envNames {
			env[name] = " \n"
		}
		setEnv(t, env)
		if _, ok, err := ConfigFromEnv(); ok || err != nil {
			t.Errorf("got ok=%v err=%v, want disabled", ok, err)
		}
	})
	t.Run("complete", func(t *testing.T) {
		setEnv(t, fullEnv())
		cfg, ok, err := ConfigFromEnv()
		want := Config{
			BaseURL:       "https://fretboard.tail9292f9.ts.net",
			Issuer:        "https://cognito-idp.us-east-2.amazonaws.com/us-east-2_8UtmIgtxM",
			ClientID:      "client-id",
			ClientSecret:  "s3cr3t-value",
			CognitoDomain: "https://fretboard-viz-login.auth.us-east-2.amazoncognito.com",
		}
		if !ok || err != nil || cfg != want {
			t.Errorf("got (%+v, %v, %v), want (%+v, true, nil)", cfg, ok, err, want)
		}
	})
	t.Run("values are trimmed", func(t *testing.T) {
		env := fullEnv()
		env["COGNITO_CLIENT_SECRET"] = "  s3cr3t-value\n"
		env["FRETBOARD_BASE_URL"] = " http://localhost:8080 "
		setEnv(t, env)
		cfg, ok, err := ConfigFromEnv()
		if !ok || err != nil || cfg.ClientSecret != "s3cr3t-value" || cfg.BaseURL != "http://localhost:8080" {
			t.Errorf("got (%+v, %v, %v)", cfg, ok, err)
		}
	})
	t.Run("any one variable alone is an error naming the rest", func(t *testing.T) {
		for _, only := range envNames {
			env := map[string]string{only: fullEnv()[only]}
			setEnv(t, env)
			cfg, ok, err := ConfigFromEnv()
			if ok || err == nil || cfg != (Config{}) {
				t.Errorf("only %s: got (%+v, %v, %v), want an error", only, cfg, ok, err)
				continue
			}
			for _, name := range envNames {
				if mentioned := strings.Contains(err.Error(), name); mentioned == (name == only) {
					t.Errorf("only %s: error %q mentions %s = %v", only, err, name, mentioned)
				}
			}
			if strings.Contains(err.Error(), fullEnv()[only]) {
				t.Errorf("only %s: error echoes the value: %v", only, err)
			}
		}
	})
	t.Run("each variable missing is an error naming it", func(t *testing.T) {
		for _, missing := range envNames {
			env := fullEnv()
			delete(env, missing)
			setEnv(t, env)
			_, ok, err := ConfigFromEnv()
			if ok || err == nil || !strings.Contains(err.Error(), missing) {
				t.Errorf("missing %s: got ok=%v err=%v", missing, ok, err)
			}
		}
	})
	t.Run("bad values", func(t *testing.T) {
		for _, tc := range []struct{ env, value string }{
			{"FRETBOARD_BASE_URL", "https://fretboard.example/"},
			{"FRETBOARD_BASE_URL", "https://fretboard.example/app"},
			{"FRETBOARD_BASE_URL", "fretboard.example"},
			{"FRETBOARD_BASE_URL", "ftp://fretboard.example"},
			{"FRETBOARD_BASE_URL", "https://user:pw@fretboard.example"},
			{"FRETBOARD_BASE_URL", "https://fretboard.example?x=1"},
			{"FRETBOARD_BASE_URL", "https://fretboard.example#frag"},
			{"FRETBOARD_BASE_URL", "https://"},
			{"COGNITO_ISSUER", "http://cognito-idp.us-east-2.amazonaws.com/us-east-2_8UtmIgtxM"},
			{"COGNITO_ISSUER", "cognito-idp.us-east-2.amazonaws.com/pool"},
			{"COGNITO_ISSUER", "https://cognito-idp.us-east-2.amazonaws.com/pool?x=1"},
			{"COGNITO_DOMAIN", "http://fretboard-viz-login.auth.us-east-2.amazoncognito.com"},
			{"COGNITO_DOMAIN", "https://fretboard-viz-login.auth.us-east-2.amazoncognito.com/"},
			{"COGNITO_DOMAIN", "fretboard-viz-login.auth.us-east-2.amazoncognito.com"},
		} {
			env := fullEnv()
			env[tc.env] = tc.value
			setEnv(t, env)
			_, ok, err := ConfigFromEnv()
			if ok || err == nil {
				t.Errorf("%s=%q: got ok=%v err=%v, want an error", tc.env, tc.value, ok, err)
				continue
			}
			if !strings.Contains(err.Error(), tc.env) {
				t.Errorf("%s=%q: error %q does not name the variable", tc.env, tc.value, err)
			}
			if strings.Contains(err.Error(), "s3cr3t-value") {
				t.Errorf("%s=%q: error leaks the client secret", tc.env, tc.value)
			}
		}
	})
	t.Run("local development over http", func(t *testing.T) {
		env := fullEnv()
		env["FRETBOARD_BASE_URL"] = "http://localhost:8080"
		setEnv(t, env)
		if cfg, ok, err := ConfigFromEnv(); !ok || err != nil || cfg.BaseURL != "http://localhost:8080" {
			t.Errorf("got (%+v, %v, %v)", cfg, ok, err)
		}
	})
}
