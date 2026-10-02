package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/nstalter/fretboard-visualizer/auth"
	"github.com/nstalter/fretboard-visualizer/store"
)

// setupAccounts registers the sign-in and saved-song routes on mux. With no Cognito settings in the environment it only reports that accounts
// are disabled, so the app behaves as it did before accounts existed. The database stays open for
// the life of the process. devUser signs everyone in as
// that email without Cognito; it is for local development only.
func setupAccounts(ctx context.Context, mux *http.ServeMux, addr, dbPath, devUser string) error {
	cfg, configured, err := auth.ConfigFromEnv()
	if err != nil {
		return err
	}
	if devUser != "" && configured {
		return errors.New("-dev-user cannot be combined with Cognito settings")
	}
	if devUser != "" && !isLoopback(addr) {
		return fmt.Errorf("-dev-user needs a loopback -addr, got %q", addr)
	}
	if devUser == "" && !configured {
		auth.Disabled(mux)
		return nil
	}

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	var a *auth.Auth
	if devUser != "" {
		a = auth.NewDev(devUser)
	} else {
		discoverCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if a, err = auth.New(discoverCtx, cfg, st); err != nil {
			st.Close()
			return err
		}
	}
	a.Register(mux)
	registerSongRoutes(mux, st, a.User, a.SameOrigin)
	return nil
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
