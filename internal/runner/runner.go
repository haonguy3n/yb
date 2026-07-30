// Package runner launches the build. With an image it runs inside that container
// (mounting the project, dl/sstate, ssh key, and extra mounts, sourcing
// oe-init-build-env, then bitbake). With no image it runs natively on the host.
//
// The final step replaces the yb process (syscall.Exec) with the build command,
// so bitbake (host) or docker (container) becomes the real foreground process and
// Ctrl+C is delivered natively — there is no yb wrapper left to die and orphan
// bitbake's server/workers (which run in their own session and can't be reached
// by forwarding signals to a child process group).
package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	"github.com/haonguy3n/yb/internal/conf"
	"github.com/haonguy3n/yb/internal/project"
)

// SSHKeyDest is where the ssh key is mounted for the builder user (matches the
// IdentityFile in the yocto-kas image's ssh config).
const SSHKeyDest = "/home/builder/.ssh/iri"

// Options controls a single run.
type Options struct {
	Image   string   // container image to run in; empty => native host build
	PokyDir string   // poky checkout dir, relative to project root
	Targets []string // bitbake targets (ignored when Shell)
	Shell   bool     // drop into bash instead of running bitbake
}

// Run executes the build (or shell) for project p, replacing the yb process. On
// success it does not return (the exec'd command's exit status becomes yb's).
func Run(p *project.Project, o Options) error {
	// DL_DIR/SSTATE_DIR must exist and be owned by the caller (a bind mount of a
	// missing path is created root-owned; native bitbake also needs them).
	for _, d := range []string{p.DLDir, p.SSTateDir} {
		if d != "" {
			_ = os.MkdirAll(d, 0o755)
		}
	}
	if o.Image == "" {
		return runHost(p, o)
	}
	return runContainer(p, o)
}

// script builds the `source oe-init-build-env … && bitbake …` (or shell) command
// run under root (WorkDir in a container, the project dir on the host).
func script(o Options, root string) string {
	init := fmt.Sprintf("source %s %s >/dev/null",
		path.Join(root, o.PokyDir, "oe-init-build-env"), path.Join(root, "build"))
	if o.Shell {
		return init + " && exec bash"
	}
	return init + " && exec bitbake " + strings.Join(o.Targets, " ")
}

func runHost(p *project.Project, o Options) error {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return err
	}
	if err := os.Chdir(p.Dir); err != nil {
		return err
	}
	return syscall.Exec(bash, []string{"bash", "-c", script(o, p.Dir)}, os.Environ())
}

func runContainer(p *project.Project, o Options) error {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return err
	}
	args := []string{"docker", "run", "--rm"}
	// A TTY gives bitbake its live progress UI (like kas-container) and delivers
	// Ctrl+C into the container. A shell always needs it; a build gets it only
	// when run interactively, so CI/piped output stays plain (and -t won't error).
	if o.Shell || interactive() {
		args = append(args, "-it")
	}
	args = append(args, "-v", p.Dir+":"+conf.WorkDir)
	seen := map[string]bool{}
	for _, d := range []string{p.DLDir, p.SSTateDir} {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		args = append(args, "-v", d+":"+d)
	}
	// A mount that doesn't exist on the host is skipped, not passed to docker:
	// docker would create it as an empty root-owned directory, and the build then
	// fails much later, deep inside bitbake, with a confusing "Permission denied"
	// on a path that looks like it exists.
	for _, m := range p.Mounts {
		host, opt := project.MountSpec(m)
		if _, err := os.Stat(host); err != nil {
			fmt.Fprintf(os.Stderr, "yb: mount %s not found on the host; building without it\n", host)
			continue
		}
		args = append(args, "-v", host+":"+host+opt)
	}
	if p.SSHKey != "" {
		if _, err := os.Stat(p.SSHKey); err == nil {
			args = append(args, "-v", p.SSHKey+":"+SSHKeyDest+":ro")
		} else {
			fmt.Fprintf(os.Stderr, "yb: ssh key %s not found; building without it\n", p.SSHKey)
		}
	}
	args = append(args, "-w", path.Join(conf.WorkDir, "build"), o.Image, "bash", "-c", script(o, conf.WorkDir))

	return syscall.Exec(docker, args, os.Environ())
}

func RunSDK(image, sdkDir, workDir string, cmd []string) error {
	docker, err := exec.LookPath("docker")
	if err != nil {
		return err
	}
	args := []string{"docker", "run", "--rm"}
	if len(cmd) == 0 || interactive() {
		args = append(args, "-it")
	}
	args = append(args,
		"-v", sdkDir+":"+sdkDir,
		"-v", workDir+":"+conf.WorkDir,
		"-w", conf.WorkDir,
		image, "bash", "-c", sdkScript(sdkDir, cmd))
	return syscall.Exec(docker, args, os.Environ())
}

func sdkScript(sdkDir string, cmd []string) string {
	src := "source " + sdkDir + "/environment-setup-* >/dev/null"
	if len(cmd) == 0 {
		return src + " && exec bash"
	}
	return src + " && exec " + strings.Join(cmd, " ")
}

// interactive reports whether both stdin and stdout are terminals, so a TTY can
// be allocated (`docker run -t` fails otherwise, e.g. when output is piped).
func interactive() bool {
	return isTTY(os.Stdin) && isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
