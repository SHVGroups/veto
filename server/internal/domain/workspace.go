package domain

import (
	"errors"
	"time"
)

// Role is a member's level of access to a workspace.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleReviewer Role = "reviewer"
	RoleViewer   Role = "viewer"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleReviewer, RoleViewer:
		return true
	}
	return false
}

// Permission is a capability checked by the API before acting.
type Permission int

const (
	// PermView reads anything in the workspace.
	PermView Permission = iota
	// PermReview decides held actions and records audit verdicts.
	PermReview
	// PermConfigure changes agents, tools, policies, packs, connections and
	// integrations.
	PermConfigure
	// PermManageMembers invites, removes and changes the roles of members.
	PermManageMembers
	// PermOwn changes workspace ownership and deletes the workspace's data.
	PermOwn
)

// Can reports whether the role grants the permission.
func (r Role) Can(p Permission) bool {
	switch r {
	case RoleOwner:
		return true
	case RoleAdmin:
		return p <= PermManageMembers
	case RoleReviewer:
		return p <= PermReview
	case RoleViewer:
		return p == PermView
	}
	return false
}

// Settings are the workspace-wide knobs an admin can change in the console.
type Settings struct {
	// ReviewTimeoutSeconds is how long a held action waits for a human before
	// it expires as stopped.
	ReviewTimeoutSeconds int `json:"review_timeout_seconds"`
	// AuditSampleRate is the share of automatically approved actions sent to
	// reviewers after the fact, between 0 and 1.
	AuditSampleRate float64 `json:"audit_sample_rate"`
	// RetentionDays is how long arguments and untrusted content are kept
	// before they are redacted. Decisions, hashes and the ledger are kept.
	RetentionDays int `json:"retention_days"`
	// UntrustedMaxChars caps how much untrusted content is shown to the model
	// per action.
	UntrustedMaxChars int `json:"untrusted_max_chars"`
	// CalibrationTarget is the agreement rate the calibration report treats
	// as safe when it suggests thresholds.
	CalibrationTarget float64 `json:"calibration_target"`
}

// DefaultSettings returns the settings a new workspace starts with.
func DefaultSettings() Settings {
	return Settings{
		ReviewTimeoutSeconds: 3600,
		AuditSampleRate:      0.02,
		RetentionDays:        90,
		UntrustedMaxChars:    12000,
		CalibrationTarget:    0.98,
	}
}

// Validate checks that every setting is in range.
func (s Settings) Validate() error {
	switch {
	case s.ReviewTimeoutSeconds < 60 || s.ReviewTimeoutSeconds > 7*24*3600:
		return errors.New("review_timeout_seconds must be between 60 and 604800")
	case s.AuditSampleRate < 0 || s.AuditSampleRate > 1:
		return errors.New("audit_sample_rate must be between 0 and 1")
	case s.RetentionDays < 1 || s.RetentionDays > 3650:
		return errors.New("retention_days must be between 1 and 3650")
	case s.UntrustedMaxChars < 0 || s.UntrustedMaxChars > 100000:
		return errors.New("untrusted_max_chars must be between 0 and 100000")
	case s.CalibrationTarget < 0.5 || s.CalibrationTarget > 1:
		return errors.New("calibration_target must be between 0.5 and 1")
	}
	return nil
}

// ReviewTimeout returns ReviewTimeoutSeconds as a duration.
func (s Settings) ReviewTimeout() time.Duration {
	return time.Duration(s.ReviewTimeoutSeconds) * time.Second
}
