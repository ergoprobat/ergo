//go:build unix

package archunit

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestStageClassesRefusesAClassThatIsNotARegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "classes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "classes", "Pipe.class"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	_, err := stageClasses(context.Background(), root, []string{"classes"}, t.TempDir())
	if compiler.CodeOf(err) != CodePath {
		t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
	}
}
