// Package config loads kas-format build configuration. It parses a kas YAML
// file, resolves its header.includes with kas deep-merge semantics, and exposes
// the machine/distro/target, repositories, and conf headers a build needs.
//
// The parser is intentionally a subset of the full kas schema — the keys our
// projects actually use (see docs/design/2026-07-06-yb.md). Unknown keys are
// ignored, so a real kas file parses unchanged.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is a fully-merged kas configuration.
type Config struct {
	Machine         string
	Distro          string
	Targets         []string
	Repos           map[string]*Repo
	LocalConfHeader map[string]string

	// Orchestration read from the file's `yb:` block (yb-only; kas ignores it).
	Version string
	Image   string
	Cache   string
	SSHKey  string
	Mounts  []string
}

// Repo is one entry under `repos:`.
type Repo struct {
	Name   string
	URL    string
	Path   string
	Commit string
	Branch string
	Layers map[string]bool // layer subdir -> enabled
}

// Dir is the repo's checkout directory relative to the project root. A url-less
// repo is the project directory itself ("."); otherwise `path`, or the repo name.
func (r *Repo) Dir() string {
	switch {
	case r.URL == "":
		return "."
	case r.Path != "":
		return r.Path
	default:
		return r.Name
	}
}

// Ref is the git ref to check out: commit if pinned, else branch.
func (r *Repo) Ref() string {
	if r.Commit != "" {
		return r.Commit
	}
	return r.Branch
}

// Layers returns every enabled layer path, relative to the project root, in a
// deterministic order. A repo with no `layers:` contributes its root as a single
// layer; otherwise each enabled subdir is one layer.
func (c *Config) Layers() []string {
	var out []string
	for _, name := range sortedKeys(c.Repos) {
		r := c.Repos[name]
		if len(r.Layers) == 0 {
			out = append(out, filepath.Clean(r.Dir()))
			continue
		}
		var subs []string
		for l, enabled := range r.Layers {
			if enabled {
				subs = append(subs, l)
			}
		}
		sort.Strings(subs)
		for _, l := range subs {
			out = append(out, filepath.Clean(filepath.Join(r.Dir(), l)))
		}
	}
	return out
}

// PokyDir returns the poky checkout dir (relative to project root): the repo
// named "poky", else any repo providing a meta-poky layer.
func (c *Config) PokyDir() (string, error) {
	if r, ok := c.Repos["poky"]; ok {
		return r.Dir(), nil
	}
	for _, name := range sortedKeys(c.Repos) {
		if _, ok := c.Repos[name].Layers["meta-poky"]; ok {
			return c.Repos[name].Dir(), nil
		}
	}
	return "", fmt.Errorf("no poky repo found (need a repo named 'poky' or one with a meta-poky layer)")
}

// FindEntry returns the single kas file in dir that carries a top-level `yb:`
// block — the buildable entry. Errors if none, or more than one, is found (pass
// an explicit file instead). Included fragments like base.yml carry no yb block.
func FindEntry(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var top struct {
			YB *rawYB `yaml:"yb"`
		}
		if yaml.Unmarshal(data, &top) == nil && top.YB != nil {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no kas file with a `yb:` block in %s (pass the file explicitly)", dir)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("multiple kas files with a `yb:` block: %s (pass one explicitly)", strings.Join(found, ", "))
	}
}

// Load parses a kas file and its transitive includes into a merged Config.
func Load(path string) (*Config, error) {
	merged, err := loadMerged(path, map[string]bool{})
	if err != nil {
		return nil, err
	}
	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, err
	}
	var rk rawKas
	if err := yaml.Unmarshal(out, &rk); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rk.toConfig(), nil
}

type rawKas struct {
	Machine         string              `yaml:"machine"`
	Distro          string              `yaml:"distro"`
	Target          stringList          `yaml:"target"`
	Repos           map[string]*rawRepo `yaml:"repos"`
	LocalConfHeader map[string]string   `yaml:"local_conf_header"`
	YB              rawYB               `yaml:"yb"`
}

// rawYB is the yb orchestration block embedded in a kas file.
type rawYB struct {
	Version string   `yaml:"version"`
	Image   string   `yaml:"image"`
	Cache   string   `yaml:"cache"`
	SSHKey  string   `yaml:"ssh_key"`
	Mounts  []string `yaml:"mounts"`
}

type rawRepo struct {
	URL    string                 `yaml:"url"`
	Path   string                 `yaml:"path"`
	Commit string                 `yaml:"commit"`
	Branch string                 `yaml:"branch"`
	Layers map[string]interface{} `yaml:"layers"`
}

func (rk *rawKas) toConfig() *Config {
	c := &Config{
		Machine:         rk.Machine,
		Distro:          rk.Distro,
		Targets:         rk.Target,
		Repos:           map[string]*Repo{},
		LocalConfHeader: rk.LocalConfHeader,
		Version:         rk.YB.Version,
		Image:           rk.YB.Image,
		Cache:           rk.YB.Cache,
		SSHKey:          rk.YB.SSHKey,
		Mounts:          rk.YB.Mounts,
	}
	for name, rr := range rk.Repos {
		r := &Repo{
			Name:   name,
			URL:    rr.URL,
			Path:   rr.Path,
			Commit: rr.Commit,
			Branch: rr.Branch,
			Layers: map[string]bool{},
		}
		for l, v := range rr.Layers {
			r.Layers[l] = layerEnabled(v)
		}
		c.Repos[name] = r
	}
	return c
}

// layerEnabled reports whether a `layers:` value means the layer is active. kas
// disables a layer when its value is falsy ("excluded"/"disabled"/false); a null
// value (the common `meta:` form) enables it.
func layerEnabled(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case bool:
		return t
	case string:
		switch t {
		case "", "excluded", "disabled", "disable", "false":
			return false
		}
		return true
	default:
		return true
	}
}

// loadMerged loads a kas file and deep-merges its includes (base first, the
// including file overriding). `stack` detects include cycles along the DFS path.
func loadMerged(path string, stack map[string]bool) (map[string]interface{}, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if stack[abs] {
		return nil, fmt.Errorf("include cycle at %s", abs)
	}
	stack[abs] = true
	defer delete(stack, abs)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var head struct {
		Header struct {
			Includes []string `yaml:"includes"`
		} `yaml:"header"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var self map[string]interface{}
	if err := yaml.Unmarshal(data, &self); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	merged := map[string]interface{}{}
	for _, inc := range head.Header.Includes {
		incPath := filepath.Join(filepath.Dir(path), inc)
		im, err := loadMerged(incPath, stack)
		if err != nil {
			return nil, err
		}
		merged = mergeMap(merged, im)
	}
	return mergeMap(merged, self), nil
}

// mergeMap deep-merges src over dst: maps merge recursively; scalars and lists
// are replaced by src (the higher-priority file). Matches kas config merging.
func mergeMap(dst, src map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if ev, ok := out[k]; ok {
			if em, eok := ev.(map[string]interface{}); eok {
				if sm, sok := v.(map[string]interface{}); sok {
					out[k] = mergeMap(em, sm)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stringList accepts a YAML scalar or sequence and decodes to []string, so
// `target: foo` and `target: [foo, bar]` both work.
type stringList []string

func (s *stringList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*s = stringList{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}
