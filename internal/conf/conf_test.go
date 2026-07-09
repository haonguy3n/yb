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
