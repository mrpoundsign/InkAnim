package inksvg

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MigrationRule defines a syntax deprecation or migration rewrite rule.
type MigrationRule struct {
	ID      string              // e.g. "movement-to-move"
	Since   string              // version the old syntax was retired, e.g. "v0.x"
	Message string              // human-readable explanation
	Detect  *regexp.Regexp      // match on a raw label
	Replace string              // regexp replacement template used by Fix (if Rewrite is nil)
	Rewrite func(string) string // optional custom rewrite function
}

// MigrationHit represents a detected syntax migration opportunity in a document.
type MigrationHit struct {
	RuleID    string
	ElementID string // filled by ParseSVG; empty from DetectMigrations
	Before    string
	After     string
}

// migrateRotRange converts "from: A; to: B" in Rot directives to relative "deg: (B - A)".
func migrateRotRange(label string) string {
	rotBlockRe := regexp.MustCompile(`(?i)\b(rot)\s*\{([^}]*)\}`)
	return rotBlockRe.ReplaceAllStringFunc(label, func(match string) string {
		sub := rotBlockRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		params := sub[2]

		fromValRe := regexp.MustCompile(`(?i)\bfrom\s*:\s*([+-]?\d+(?:\.\d+)?(?:deg)?)`)
		toValRe := regexp.MustCompile(`(?i)\bto\s*:\s*([+-]?\d+(?:\.\d+)?(?:deg)?)`)

		fromMatch := fromValRe.FindStringSubmatch(params)
		toMatch := toValRe.FindStringSubmatch(params)
		if len(fromMatch) == 0 && len(toMatch) == 0 {
			return match
		}

		var fromVal, toVal float64
		if len(fromMatch) > 0 {
			fromVal, _ = strconv.ParseFloat(strings.TrimSuffix(fromMatch[1], "deg"), 64)
		}
		if len(toMatch) > 0 {
			toVal, _ = strconv.ParseFloat(strings.TrimSuffix(toMatch[1], "deg"), 64)
		}
		delta := toVal - fromVal

		fromLoc := fromValRe.FindStringIndex(params)
		toLoc := toValRe.FindStringIndex(params)

		degStr := fmt.Sprintf("deg: %g", delta)
		switch {
		case len(fromLoc) > 0 && len(toLoc) > 0:
			if fromLoc[0] < toLoc[0] {
				// from is first: replace from with degStr, then remove to
				params = params[:fromLoc[0]] + degStr + params[fromLoc[1]:]
				removeToRe := regexp.MustCompile(`(?i)\s*[;,]?\s*\bto\s*:\s*[+-]?\d+(?:\.\d+)?(?:deg)?`)
				params = removeToRe.ReplaceAllString(params, "")
			} else {
				// to is first: replace to with degStr, then remove from
				params = params[:toLoc[0]] + degStr + params[toLoc[1]:]
				removeFromRe := regexp.MustCompile(`(?i)\s*[;,]?\s*\bfrom\s*:\s*[+-]?\d+(?:\.\d+)?(?:deg)?`)
				params = removeFromRe.ReplaceAllString(params, "")
			}
		case len(fromLoc) > 0:
			params = params[:fromLoc[0]] + degStr + params[fromLoc[1]:]
		case len(toLoc) > 0:
			params = params[:toLoc[0]] + degStr + params[toLoc[1]:]
		}

		braceIdx := strings.Index(match, "{")
		return match[:braceIdx+1] + params + "}"
	})
}

// migrateAngleToDeg converts "angle: <val>" in Rot and Color directives to "deg: <val>".
func migrateAngleToDeg(label string) string {
	directiveBlockRe := regexp.MustCompile(`(?i)\b(rot|color)\s*\{([^}]*)\}`)
	return directiveBlockRe.ReplaceAllStringFunc(label, func(match string) string {
		sub := directiveBlockRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		angleRe := regexp.MustCompile(`(?i)\bangle(\s*:)`)
		replacedParams := angleRe.ReplaceAllString(sub[2], "deg$1")
		braceIdx := strings.Index(match, "{")
		return match[:braceIdx+1] + replacedParams + "}"
	})
}

var migrationRules = []MigrationRule{
	{
		ID:      "movement-to-move",
		Since:   "v0.4.0",
		Message: "\"Movement\" was renamed to \"Move\"",
		Detect:  regexp.MustCompile(`(?i)\bmovement(\s*\{)`),
		Replace: "Move$1",
	},
	{
		ID:      "motion-to-move",
		Since:   "v0.4.0",
		Message: "\"Motion\" was renamed to \"Move\"",
		Detect:  regexp.MustCompile(`(?i)\bmotion(\s*\{)`),
		Replace: "Move$1",
	},
	{
		ID:      "rot-range-to-deg",
		Since:   "v0.4.0",
		Message: "\"from: / to:\" in Rot directives was migrated to relative \"deg:\"",
		Detect:  regexp.MustCompile(`(?i)\brot\s*\{[^}]*\b(?:from|to)\s*:`),
		Rewrite: migrateRotRange,
	},
	{
		ID:      "angle-to-deg",
		Since:   "v0.4.0",
		Message: "\"angle:\" in Rot and Color directives was migrated to \"deg:\"",
		Detect:  regexp.MustCompile(`(?i)\b(?:rot|color)\s*\{[^}]*\bangle\s*:`),
		Rewrite: migrateAngleToDeg,
	},
}

// migrateLabel applies every rule in registry order; returns the rewritten label.
func migrateLabel(label string) string {
	res := label
	for _, rule := range migrationRules {
		if rule.Rewrite != nil {
			res = rule.Rewrite(res)
		} else if rule.Detect != nil && rule.Replace != "" {
			res = rule.Detect.ReplaceAllString(res, rule.Replace)
		}
	}
	return res
}

// MigrateLabel applies all registered migration rules and returns the modernized label.
func MigrateLabel(label string) string {
	return migrateLabel(label)
}

// DetectMigrations returns one hit per matching rule with Before/After (no ElementID).
func DetectMigrations(label string) []MigrationHit {
	var hits []MigrationHit
	for _, rule := range migrationRules {
		if rule.Detect.MatchString(label) {
			var after string
			if rule.Rewrite != nil {
				after = rule.Rewrite(label)
			} else {
				after = rule.Detect.ReplaceAllString(label, rule.Replace)
			}
			hits = append(hits, MigrationHit{
				RuleID: rule.ID,
				Before: label,
				After:  after,
			})
		}
	}
	return hits
}
