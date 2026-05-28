package studio

// naming.go — small predicates for the framework's naming
// conventions. The framework's docs/conventions.md is the source
// of truth: events / states / actions are UPPER_SNAKE_CASE;
// entity types are PascalCase; permissions are dotted.lowercase;
// adapter and role names are lowercase, single-word or
// snake_case. These helpers keep the validation logic readable
// in blueprint.go and the generators.

// isLowerHyphen reports whether s is non-empty, starts with a
// lowercase letter, and contains only [a-z0-9-].
func isLowerHyphen(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		case r == '-':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// isPascalCase reports whether s starts with an uppercase letter
// and contains only ASCII letters and digits.
func isPascalCase(s string) bool {
	if s == "" {
		return false
	}
	if !(s[0] >= 'A' && s[0] <= 'Z') {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// isUpperSnake reports whether s is non-empty and contains only
// uppercase letters, digits, and underscores, starting with a
// letter.
func isUpperSnake(s string) bool {
	if s == "" {
		return false
	}
	if !(s[0] >= 'A' && s[0] <= 'Z') {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// isSnakeCase reports whether s is non-empty, starts with a
// lowercase letter, and contains only [a-z0-9_].
func isSnakeCase(s string) bool {
	if s == "" {
		return false
	}
	if !(s[0] >= 'a' && s[0] <= 'z') {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// isDottedLower reports whether s is a dotted.lowercase
// identifier — one or more dot-separated segments, each
// non-empty and matching [a-z][a-z0-9_-]*. Segments may contain
// hyphens (the domain field is hyphenated, e.g. "refund-flow")
// or underscores (the action-name component is snake_case,
// e.g. "issue_refund"); permissions like "refund-flow.issue_refund"
// must validate cleanly.
func isDottedLower(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			seg := s[start:i]
			if !isDottedSegment(seg) {
				return false
			}
			start = i + 1
		}
	}
	return true
}

// isDottedSegment reports whether seg is a non-empty
// permission-segment: starts with a lowercase letter and
// contains only lowercase letters, digits, hyphens, and
// underscores.
func isDottedSegment(seg string) bool {
	if seg == "" {
		return false
	}
	if !(seg[0] >= 'a' && seg[0] <= 'z') {
		return false
	}
	for _, r := range seg {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// camelToSnake converts PascalCase / camelCase to snake_case.
// "OrderItem" -> "order_item", "Order" -> "order". Used by
// generators that derive event/state names from an Entity
// (e.g. "Order" + "READY" -> "ORDER_READY").
func camelToSnake(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				out = append(out, '_')
			}
			out = append(out, c+('a'-'A'))
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

// stripHyphens turns "refund-flow" into "refundflow". Used to
// derive the adapter name from the domain — adapter names per the
// framework's conventions are single-word lowercase, while the
// YAML's domain field accepts hyphens.
func stripHyphens(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// goExportName turns a hyphenated lowercase string into
// PascalCase: "refund-flow" -> "RefundFlow". Used to derive Go
// identifier names from domain-level names.
func goExportName(s string) string {
	out := make([]byte, 0, len(s))
	upper := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || c == '_' {
			upper = true
			continue
		}
		if upper && c >= 'a' && c <= 'z' {
			out = append(out, c-('a'-'A'))
		} else {
			out = append(out, c)
		}
		upper = false
	}
	return string(out)
}

// goExportFromUpperSnake turns "ORDER_READY_FOR_REFUND" into
// "OrderReadyForRefund". Used to derive Go const names from
// UPPER_SNAKE_CASE event/state/action names.
func goExportFromUpperSnake(s string) string {
	out := make([]byte, 0, len(s))
	upper := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			upper = true
			continue
		}
		if upper {
			// Already uppercase in UPPER_SNAKE_CASE.
			out = append(out, c)
			upper = false
			continue
		}
		// Convert subsequent letters to lowercase.
		if c >= 'A' && c <= 'Z' {
			out = append(out, c+('a'-'A'))
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

// goExportFromPerm turns "refund-flow.issue_refund.approve" into
// "RefundFlowIssueRefundApprove". Used to derive Go const names
// from dotted.lowercase permission strings.
func goExportFromPerm(s string) string {
	out := make([]byte, 0, len(s))
	upper := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || c == '_' || c == '.' {
			upper = true
			continue
		}
		if upper && c >= 'a' && c <= 'z' {
			out = append(out, c-('a'-'A'))
		} else {
			out = append(out, c)
		}
		upper = false
	}
	return string(out)
}

// isGoIdent reports whether s is a valid Go identifier:
// non-empty, starts with a letter or underscore, contains only
// letters, digits, and underscores. Used to validate user-provided
// package names before inserting them into generated source.
func isGoIdent(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_':
		default:
			return false
		}
	}
	return true
}
