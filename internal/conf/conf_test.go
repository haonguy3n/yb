package conf

import (
	"strings"
	"testing"

	"github.com/haonguy3n/yb/internal/config"
)

func TestLocalConfOmitsUnsetCacheDirs(t *testing.T) {
	got := LocalConf(&config.Config{Machine: "qemu", Distro: "poky"}, "", "", 4)
	if strings.Contains(got, "DL_DIR") || strings.Contains(got, "SSTATE_DIR") {
		t.Fatalf("unset cache dirs should be omitted:\n%s", got)
	}
}

func TestLocalConfThreadDefaultsYieldToConfig(t *testing.T) {
	bare := LocalConf(&config.Config{Machine: "qemu", Distro: "poky"}, "", "", 12)
	if !strings.Contains(bare, `BB_NUMBER_THREADS ??= "12"`) || !strings.Contains(bare, `PARALLEL_MAKE ??= "-j 12"`) {
		t.Fatalf("thread defaults missing when the config sets none:\n%s", bare)
	}
	c := &config.Config{Machine: "qemu", Distro: "poky", LocalConfHeader: map[string]string{
		"build": "BB_NUMBER_THREADS = \"4\"\nPARALLEL_MAKE = \"-j 4\"\n",
	}}
	got := LocalConf(c, "", "", 12)
	if strings.Contains(got, "??= \"12\"") || strings.Contains(got, "-j 12") {
		t.Fatalf("defaults must not be added when the config sets the threads:\n%s", got)
	}
}
