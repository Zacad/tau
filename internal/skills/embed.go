package skills

import "embed"

//go:embed builtin/*/SKILL.md
var builtinFS embed.FS

// BuiltinFS returns the embedded filesystem containing built-in skills.
// Each built-in skill is a directory under builtin/ with a SKILL.md file.
func BuiltinFS() embed.FS {
	return builtinFS
}
