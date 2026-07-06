// Package project holds the resolved orchestration for one build tree — which
// container image/version to run, the cache dir, ssh key, and extra bind mounts.
// These come from the `yb:` block of the kas file (see internal/config).
package project

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/haonguy3n/yb/internal/config"
)

// Defaults for the yb block's dl/sstate when unset.
const (
	DefaultDL     = "/srv/yocto-cache/downloads"
	DefaultSState = "/srv/yocto-cache/sstate"
)

// Project is the resolved orchestration config for one build tree.
type Project struct {
	Version   string   // Yocto release; yb builds an aligned image
	Image     string   // optional: use this image instead of building one
	DLDir     string   // DL_DIR (mounted into the container)
	SSTateDir string   // SSTATE_DIR (mounted into the container)
	SSHKey    string   // mounted read-only for private git
	Mounts    []string // extra bind mounts ("host/path" or "host/path:ro")
	Dir       string   // absolute project root
}

// New builds a Project from a parsed config and the project directory, applying
// defaults and expanding "~" in paths.
func New(dir string, c *config.Config) (*Project, error) {
	p := &Project{
		Version:   c.Version,
		Image:     c.Image,
		DLDir:     expandHome(c.DLDir),
		SSTateDir: expandHome(c.SSTateDir),
		SSHKey:    expandHome(c.SSHKey),
	}
	for _, m := range c.Mounts {
		p.Mounts = append(p.Mounts, expandHome(m))
	}
	if p.DLDir == "" {
		p.DLDir = DefaultDL
	}
	if p.SSTateDir == "" {
		p.SSTateDir = DefaultSState
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	p.Dir = abs
	return p, nil
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}
