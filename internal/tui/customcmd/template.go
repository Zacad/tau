package customcmd

import "strings"

// SkillResolver looks up a skill by name and returns its full content,
// or empty string if not found.
type SkillResolver func(name string) string

// ProcessTemplate substitutes placeholders in a custom command template.
// Supported placeholders:
//
//	$ARGUMENTS   — replaced with the full args string
//	$1, $2, $3   — replaced with positional arguments (split by whitespace)
//	$SKILL:name  — replaced with the full content of the named skill
//
// Missing positional arguments are replaced with empty strings.
// Unknown skills in $SKILL:name are replaced with an error notice.
// If resolver is nil, $SKILL:name placeholders are left unchanged.
func ProcessTemplate(template string, args string, resolver SkillResolver) string {
	result := strings.ReplaceAll(template, "$ARGUMENTS", args)

	// Resolve $SKILL:name placeholders
	if resolver != nil {
		result = resolveSkillPlaceholders(result, resolver)
	}

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

// resolveSkillPlaceholders finds all $SKILL:name patterns and resolves them.
func resolveSkillPlaceholders(template string, resolver SkillResolver) string {
	// Process all $SKILL:name occurrences
	for {
		idx := strings.Index(template, "$SKILL:")
		if idx == -1 {
			break
		}

		start := idx
		nameStart := idx + 7 // length of "$SKILL:"

		// Find the end of the skill name (space or newline)
		nameEnd := len(template)
		for i := nameStart; i < len(template); i++ {
			c := template[i]
			if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
				nameEnd = i
				break
			}
		}

		name := template[nameStart:nameEnd]
		content := resolver(name)
		if content == "" {
			content = "[Skill " + name + " not found]"
		}

		template = template[:start] + content + template[nameEnd:]
	}
	return template
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
