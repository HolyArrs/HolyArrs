package main

import (
	"context"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// startAuth makes the sign-in guard from the environment, with the owner and
// API key it asks for. serve calls it before the routes are served, so the
// wizard never gets the chance to hand a fresh install to its first visitor.
func startAuth(ctx context.Context, s *database.Store, vault *secrets.Vault, c config, unmapped bool) (*accounts.Auth, error) {
	sso, err := accounts.SSOFromEnv(c.URLBase)
	if err != nil {
		return nil, err
	}
	setup, err := accounts.ReadEnvSetup()
	if err != nil {
		return nil, err
	}
	au := accounts.New(s, accounts.Options{Vault: vault, SafeMode: c.SafeMode, SSO: sso, ConfigUnmapped: unmapped})
	if err := au.ApplyEnvSetup(ctx, setup); err != nil {
		au.Close()
		return nil, err
	}
	return au, nil
}

// authRefused holds the plain failure page for an owner or API key variable a
// person has to fix, instead of exiting into a restart loop that would only
// meet the same refusal. Other errors (single sign-on, the database) exit as
// before.
func authRefused(err error, hold func(error) error) error {
	var es *accounts.EnvSetupError
	if errors.As(err, &es) {
		return hold(err)
	}
	return err
}
