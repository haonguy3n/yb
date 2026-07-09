// Package repo checks out the git repositories named in a kas config to their
// pinned refs. Checkout runs on the host (git is a host tool); when an ssh key
// is configured it is threaded through GIT_SSH_COMMAND for private repos.
package repo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"

	"github.com/haonguy3n/yb/internal/config"
)

// Checkout ensures every url-backed repo exists at projectDir and is checked out
// at its pinned ref. A url-less repo (the project itself) is skipped. When force
// is set, yb fetches and hard-checks-out the pinned ref (a branch is reset to its
// latest origin tip), discarding local changes; otherwise it checks out gently
// and fetches only when the ref is not already present.
//
// Repos are checked out concurrently. Each repo's git output is buffered and only
// surfaced (as part of the returned error) when that repo fails, so a successful
// run is quiet and a failed run shows a clean per-repo diagnostic instead of
// interleaved progress from several clones.
func Checkout(projectDir string, repos map[string]*config.Repo, sshKey string, force bool) error {
	env := os.Environ()
	if sshKey != "" {
		if _, err := os.Stat(sshKey); err == nil {
			env = append(env, "GIT_SSH_COMMAND=ssh -i "+sshKey+" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new")
		}
	}

	// Collect url-backed repos in deterministic order so error ordering is stable.
	type job struct {
		name string
		r    *config.Repo
	}
	var jobs []job
	for _, name := range sortedRepoNames(repos) {
		r := repos[name]
		if r.URL == "" {
			continue // the project dir itself; nothing to fetch
		}
		if r.Ref() == "" {
			return fmt.Errorf("repo %q: no ref (set commit or branch)", name)
		}
		jobs = append(jobs, job{name, r})
	}

	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j job) {
			defer wg.Done()
			errs[i] = checkoutOne(env, projectDir, j.name, j.r, force)
		}(i, j)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// checkoutOne clones (if missing) and checks out a single repo at its pinned ref.
// All git stdout/stderr is captured into the returned error so a concurrent run
// stays quiet on success and yields a self-contained diagnostic on failure.
func checkoutOne(env []string, projectDir, name string, r *config.Repo, force bool) error {
	ref := r.Ref()
	dir := filepath.Join(projectDir, r.Dir())

	if !isGitRepo(dir) {
		if out, err := runCapture(env, "", "git", "clone", r.URL, dir); err != nil {
			return fmt.Errorf("clone %s: %w\n%s", name, err, out)
		}
	}

	if force {
		if out, err := runCapture(env, dir, "git", "fetch", "--all", "--tags"); err != nil {
			return fmt.Errorf("fetch %s: %w\n%s", name, err, out)
		}
		// A branch pin tracks its latest origin tip; a commit pin is exact.
		// Either way, force past any local changes.
		if r.Commit == "" && r.Branch != "" {
			if out, err := runCapture(env, dir, "git", "checkout", "-f", "-B", r.Branch, "origin/"+r.Branch); err != nil {
				return fmt.Errorf("force checkout %s @ %s: %w\n%s", name, r.Branch, err, out)
			}
			return nil
		}
		if out, err := runCapture(env, dir, "git", "checkout", "-f", ref); err != nil {
			return fmt.Errorf("force checkout %s @ %s: %w\n%s", name, ref, err, out)
		}
		return nil
	}

	// Default: fetch only when the ref isn't already present locally. After
	// `git clone` a branch exists only as the remote-tracking ref origin/<branch>,
	// not as a local branch, so check both — otherwise every first run fetches
	// a repo that was just cloned.
	if !refPresent(env, dir, ref, r.Commit == "" && r.Branch != "") {
		if out, err := runCapture(env, dir, "git", "fetch", "--all", "--tags"); err != nil {
			return fmt.Errorf("fetch %s: %w\n%s", name, err, out)
		}
	}
	if out, err := runCapture(env, dir, "git", "checkout", "-q", ref); err != nil {
		return fmt.Errorf("checkout %s @ %s: %w\n%s", name, ref, err, out)
	}
	return nil
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// refPresent reports whether ref names an object already in the local repo's
// object store, so we can skip fetching when the pinned ref is already there.
// For a branch, also probes the remote-tracking ref origin/<branch>, which is
// what `git clone` creates — the local branch may not exist until first checkout.
// Output is suppressed: this is a probe, and failure is expected, not an error
// the user needs to see.
func refPresent(env []string, dir, ref string, isBranch bool) bool {
	probe := func(args ...string) bool {
		_, err := runCapture(env, dir, append([]string{"git", "cat-file", "-t"}, args...)...)
		return err == nil
	}
	if probe(ref) {
		return true
	}
	if isBranch {
		return probe("origin/" + ref)
	}
	return false
}

// runCapture runs a command with the given env/dir, capturing stdout+stderr into
// a single buffer. The captured output is returned (empty on success); callers
// embed it into an error on failure. The first arg is the command name.
func runCapture(env []string, dir string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func sortedRepoNames(repos map[string]*config.Repo) []string {
	names := make([]string, 0, len(repos))
	for n := range repos {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
