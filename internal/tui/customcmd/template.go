package customcmd

import "strings"

// ProcessTemplate substitutes placeholders in a custom command template.
// Supported placeholders:
//
//	$ARGUMENTS — replaced with the full args string
//	$1, $2, $3 — replaced with positional arguments (split by whitespace)
//
// Missing positional arguments are replaced with empty strings.
func ProcessTemplate(template string, args string) string {
	result := strings.ReplaceAll(template, "$ARGUMENTS", args)

	positional := splitArgs(args)
	for i := 1; i <= 3; i++ {
		placeholder := "$" + itoa(i)
		value := ""
		if i-1 < len(positional) {
			value = positional[i-1]
		}
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

func splitArgs(args string) []string {
	if args == "" {
		return nil
	}
	return strings.Fields(args)
}

func itoa(n int) string {
	switch n {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	default:
		return ""
	}
}
