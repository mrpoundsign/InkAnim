package inksvg

import (
	"regexp"
)

// MigrationRule defines a syntax deprecation or migration rewrite rule.
type MigrationRule struct {
	ID      string         // e.g. "movement-to-move"
	Since   string         // version the old syntax was retired, e.g. "v0.x"
	Message string         // human-readable explanation
	Detect  *regexp.Regexp // match on a raw label
	Replace string         // regexp replacement template used by Fix
}

// MigrationHit represents a detected syntax migration opportunity in a document.
type MigrationHit struct {
	RuleID    string
	ElementID string // filled by ParseSVG; empty from DetectMigrations
	Before    string
	After     string
}

var migrationRules = []MigrationRule{
	{
		ID:      "movement-to-move",
		Since:   "v0.4.0",
		Message: "\"Movement\" was renamed to \"Move\"",
		Detect:  regexp.MustCompile(`(?i)\bmovement(\s*\{)`),
		Replace: "Move$1",
	},
}

// migrateLabel applies every rule's Detect.ReplaceAllString(label, Replace) in registry order; returns the rewritten label.
func migrateLabel(label string) string {
	res := label
	for _, rule := range migrationRules {
		res = rule.Detect.ReplaceAllString(res, rule.Replace)
	}
	return res
}

// DetectMigrations returns one hit per matching rule with Before/After (no ElementID).
func DetectMigrations(label string) []MigrationHit {
	var hits []MigrationHit
	for _, rule := range migrationRules {
		if rule.Detect.MatchString(label) {
			after := rule.Detect.ReplaceAllString(label, rule.Replace)
			hits = append(hits, MigrationHit{
				RuleID: rule.ID,
				Before: label,
				After:  after,
			})
		}
	}
	return hits
}
