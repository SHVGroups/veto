package app

import (
	"context"
	"fmt"
)

// RekeyReport counts the secrets resealed by Rekey.
type RekeyReport struct {
	SigningKeys  int
	Connections  int
	Webhooks     int
	Integrations int
}

// Total returns the number of resealed secrets.
func (r RekeyReport) Total() int { return r.SigningKeys + r.Connections + r.Webhooks + r.Integrations }

// Rekey reseals every stored secret that is not yet under the current master
// key. Run it after configuring a new VETO_MASTER_KEY with the old one in
// VETO_MASTER_KEY_PREVIOUS; once it reports zero remaining, the previous key
// can be removed.
func (a *App) Rekey(ctx context.Context) (RekeyReport, error) {
	var rep RekeyReport
	keys, err := a.Store.AllSealedSigningKeys(ctx)
	if err != nil {
		return rep, err
	}
	for _, k := range keys {
		if a.Keyring.SealedWithCurrent(k.PrivateKeyEnc) {
			continue
		}
		sealed, err := a.Keyring.Reseal(k.PrivateKeyEnc, "signing_key:"+k.ID)
		if err != nil {
			return rep, fmt.Errorf("signing key %s: %w", k.ID, err)
		}
		if err := a.Store.UpdateSealedSigningKey(ctx, k.ID, sealed); err != nil {
			return rep, err
		}
		rep.SigningKeys++
	}
	conns, err := a.Store.AllConnectionSecrets(ctx)
	if err != nil {
		return rep, err
	}
	for _, c := range conns {
		if a.Keyring.SealedWithCurrent(c.SecretEnc) {
			continue
		}
		sealed, err := a.Keyring.Reseal(c.SecretEnc, "connection:"+c.ID)
		if err != nil {
			return rep, fmt.Errorf("connection %s: %w", c.ID, err)
		}
		if err := a.Store.UpdateConnectionSecret(ctx, c.ID, sealed); err != nil {
			return rep, err
		}
		rep.Connections++
	}
	hooks, err := a.Store.AllWebhookSecrets(ctx)
	if err != nil {
		return rep, err
	}
	for _, w := range hooks {
		if a.Keyring.SealedWithCurrent(w.SecretEnc) {
			continue
		}
		sealed, err := a.Keyring.Reseal(w.SecretEnc, "webhook:"+w.ID)
		if err != nil {
			return rep, fmt.Errorf("webhook %s: %w", w.ID, err)
		}
		if err := a.Store.UpdateWebhookSecret(ctx, w.ID, sealed); err != nil {
			return rep, err
		}
		rep.Webhooks++
	}
	integs, err := a.Store.AllIntegrationSecrets(ctx)
	if err != nil {
		return rep, err
	}
	for _, in := range integs {
		if a.Keyring.SealedWithCurrent(in.SecretEnc) {
			continue
		}
		sealed, err := a.Keyring.Reseal(in.SecretEnc, "integration:"+in.WorkspaceID+":"+in.Kind)
		if err != nil {
			return rep, fmt.Errorf("integration %s/%s: %w", in.WorkspaceID, in.Kind, err)
		}
		if err := a.Store.UpdateIntegrationSecret(ctx, in.WorkspaceID, in.Kind, sealed); err != nil {
			return rep, err
		}
		rep.Integrations++
	}
	return rep, nil
}
