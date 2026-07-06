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
// at its pinned ref. A url-less repo (the project itself) is skipped. When
// dryRun is set, planned git commands are logged and nothing runs.
func Checkout(projectDir string, repos map[string]*config.Repo, sshKey string, dryRun bool, log Logf) error {
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

		if dryRun {
			if !isGitRepo(dir) {
				log("git clone %s %s", r.URL, r.Dir())
			}
			log("git -C %s checkout %s", r.Dir(), ref)
			continue
		}

		if !isGitRepo(dir) {
			log("cloning %s -> %s", r.URL, r.Dir())
			if err := run(env, "", "git", "clone", r.URL, dir); err != nil {
				return fmt.Errorf("clone %s: %w", name, err)
			}
		}
		// Fetch only when the ref isn't already present locally.
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
