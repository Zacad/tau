package skills

import (
	"fmt"
	"strings"
)

// FormatForPrompt formats a list of skills as progressive disclosure XML
// for inclusion in the system prompt. Only name and description are exposed.
// Skills with DisableModelInvocation=true are excluded.
//
// Output format (ARCHITECTURE.md §4.2):
//
//	<skills>
//	<skill name="skill-name" description="What this skill does"/>
//	</skills>
func FormatForPrompt(skills []*Skill) string {
	var sb strings.Builder
	sb.WriteString("<skills>\n")
	for _, s := range skills {
		if s.DisableModelInvocation {
			continue
		}
		fmt.Fprintf(&sb, `<skill name="%s" description="%s"/>`+"\n",
			escapeXML(s.Name), escapeXML(s.Description))
	}
	sb.WriteString("</skills>")
	return sb.String()
}

// escapeXML escapes the minimal set of characters needed for safe XML
// attribute values: &, <, >, and ".
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
