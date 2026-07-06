// Package repo checks out the git repositories named in a kas config to their
// pinned refs. Checkout runs on the host (git is a host tool); when an ssh key
// is configured it is threaded through GIT_SSH_COMMAND for private repos.
package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/haonguy3n/yb/internal/config"
)

// Logf receives one line of progress per action.
type Logf func(format string, args ...any)

// Checkout ensures every url-backed repo exists at projectDir and is checked out
// at its pinned ref. A url-less repo (the project itself) is skipped. When force
// is set, yb fetches and hard-checks-out the pinned ref (a branch is reset to its
// latest origin tip), discarding local changes; otherwise it checks out gently
// and fetches only when the ref is not already present.
func Checkout(projectDir string, repos map[string]*config.Repo, sshKey string, force bool, log Logf) error {
	env := os.Environ()
	if sshKey != "" {
		if _, err := os.Stat(sshKey); err == nil {
			env = append(env, "GIT_SSH_COMMAND=ssh -i "+sshKey+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new")
		}
	}

	names := make([]string, 0, len(repos))
	for n := range repos {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		r := repos[name]
		if r.URL == "" {
			continue // the project dir itself; nothing to fetch
		}
		ref := r.Ref()
		if ref == "" {
			return fmt.Errorf("repo %q: no ref (set commit or branch)", name)
		}
		dir := filepath.Join(projectDir, r.Dir())

		if !isGitRepo(dir) {
			log("cloning %s -> %s", r.URL, r.Dir())
			if err := run(env, "", "git", "clone", r.URL, dir); err != nil {
				return fmt.Errorf("clone %s: %w", name, err)
			}
		}

		if force {
			log("fetching %s", name)
			if err := run(env, dir, "git", "fetch", "--all", "--tags"); err != nil {
				return fmt.Errorf("fetch %s: %w", name, err)
			}
			// A branch pin tracks its latest origin tip; a commit pin is exact.
			// Either way, force past any local changes.
			if r.Commit == "" && r.Branch != "" {
				log("force %s -> origin/%s", name, r.Branch)
				if err := run(env, dir, "git", "checkout", "-f", "-B", r.Branch, "origin/"+r.Branch); err != nil {
					return fmt.Errorf("force checkout %s @ %s: %w", name, r.Branch, err)
				}
			} else if err := run(env, dir, "git", "checkout", "-f", ref); err != nil {
				return fmt.Errorf("force checkout %s @ %s: %w", name, ref, err)
			} else {
				log("force checkout %s @ %s", name, short(ref))
			}
			continue
		}

		// Default: fetch only when the ref isn't already present locally.
		if run(env, dir, "git", "cat-file", "-t", ref) != nil {
			log("fetching %s", name)
			if err := run(env, dir, "git", "fetch", "--all", "--tags"); err != nil {
				return fmt.Errorf("fetch %s: %w", name, err)
			}
		}
		log("checkout %s @ %s", name, short(ref))
		if err := run(env, dir, "git", "checkout", "-q", ref); err != nil {
			return fmt.Errorf("checkout %s @ %s: %w", name, ref, err)
		}
	}
	return nil
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func run(env []string, dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func short(ref string) string {
	if len(ref) == 40 {
		return ref[:12]
	}
	return ref
}
