package ui

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:assets/foundation
var foundation embed.FS

//go:embed assets/SKILL.md
var skill string

//go:embed assets/craft.md
var craft string

// Skill is the ui Skill as the binary ships it: the method, then the craft.
func Skill() string { return skill + "\n" + craft }

// Init lays the foundation into <root>/<dir>: tokens.css, palettes.css, layout.css and two
// example components. An existing file is never touched; it returns what it created.
func Init(root, dir string) ([]string, error) {
	var made []string
	err := fs.WalkDir(foundation, "assets/foundation", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		r, _ := filepath.Rel("assets/foundation", p)
		dst := filepath.Join(root, dir, r)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		b, err := foundation.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		made = append(made, rel(root, dst))
		return nil
	})
	return made, err
}
