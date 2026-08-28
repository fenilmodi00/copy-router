package auth

import (
	"context"
)

// PlaygroundAPIKeyName is the stable display name for the routing key
// attributed to dashboard playground inference turns.
const PlaygroundAPIKeyName = "Playground"

// ResolvePlaygroundAPIKeyID returns a routing api_key_id for dashboard
// playground attribution. Reuses the named key when present; otherwise creates
// one. The raw token is discarded — playground auth is the dashboard session.
func (s *Service) ResolvePlaygroundAPIKeyID(ctx context.Context, installationID string) (string, error) {
	if installationID == "" || s.apiKeys == nil {
		return "", nil
	}
	keys, err := s.apiKeys.ListForInstallation(ctx, installationID)
	if err != nil {
		return "", err
	}
	for _, k := range keys {
		if k == nil || k.Scope.Normalized() != ScopeRouting {
			continue
		}
		if k.Name != nil && *k.Name == PlaygroundAPIKeyName {
			return k.ID, nil
		}
	}
	name := PlaygroundAPIKeyName
	key, _, err := s.IssueScopedAPIKey(ctx, installationID, ScopeRouting, &name, nil)
	if err != nil {
		return "", err
	}
	return key.ID, nil
}

// NotifyRoutedRequest stamps first_request_served_at for dashboard onboarding.
func (s *Service) NotifyRoutedRequest(installationID string) {
	s.fireMarkFirstRequestServed(installationID)
}
