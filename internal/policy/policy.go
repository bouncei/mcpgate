package policy

// Allowed reports whether tool is permitted by the allowlist.
// Deny by default: an empty/nil allowlist permits nothing. "*" permits all.
func Allowed(allow []string, tool string) bool {
	for _, a := range allow {
		if a == "*" || a == tool {
			return true
		}
	}
	return false
}
