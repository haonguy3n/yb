// Package project loads yb.yaml, the orchestration layer that wraps a kas build:
// which container image to run, where the shared cache lives, the ssh key, and
// any extra bind mounts (e.g. HAB signing keys). It is yb-only; kas ignores it.
package project

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultCache is used when yb.yaml sets no cache.
const DefaultCache = "/srv/yocto-cache"

// Project is the resolved orchestration config for one build tree.
type Project struct {
	KasFile string   `yaml:"kas_file"`
	Version string   `yaml:"version"` // Yocto release; yb builds an aligned image
	Image   string   `yaml:"image"`   // optional: use this image instead of building one
	Cache   string   `yaml:"cache"`
	SSHKey  string   `yaml:"ssh_key"`
	Mounts  []string `yaml:"mounts"` // "host/path" or "host/path:ro"

	Dir string `yaml:"-"` // absolute project root
}

// Load reads dir/yb.yaml (optional) and applies defaults. dir becomes the
// absolute project root.
func Load(dir string) (*Project, error) {
	p := &Project{}
	data, err := os.ReadFile(filepath.Join(dir, "yb.yaml"))
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, p); err != nil {
			return nil, err
		}
	case !os.IsNotExist(err):
		return nil, err
	}

	if p.Cache == "" {
		p.Cache = DefaultCache
	}
	p.SSHKey = expandHome(p.SSHKey)
	for i, m := range p.Mounts {
		p.Mounts[i] = expandHome(m)
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
