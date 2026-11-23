package main_test

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMakeFat(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("works on darwin only")
	}

	// Make a directory to work in.
	dir, err := ioutil.TempDir("", "makefat")
	if err != nil {
		t.Fatalf("could not create directory: %v", err)
	}
	defer os.RemoveAll(dir)

	// List files we're working with.
	src := filepath.Join(dir, "test.go")
	amd64 := filepath.Join(dir, "amd64")
	arm64 := filepath.Join(dir, "arm64")
	fat := filepath.Join(dir, "fat")

	// Create test source.
	f, err := os.Create(src)
	if err != nil {
		t.Fatalf("could not create source file: %v", err)
	}
	f.Write([]byte(`
package main
import "fmt"
func main() {
	fmt.Println("hello world")
}
`))
	f.Close()

	// Compile test code in both amd64 and arm64.
	cmd := exec.Command("go", "build", "-o", amd64, src)
	cmd.Env = append(os.Environ(), "GOARCH=amd64")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("could not build amd64 target: %v\n%s\n", err, string(out))
	}
	cmd = exec.Command("go", "build", "-o", arm64, src)
	cmd.Env = append(os.Environ(), "GOARCH=arm64")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("could not build arm64 target: %v\n%s\n", err, string(out))
	}

	// Build fat binary.
	cmd = exec.Command("go", "run", "makefat.go", fat, amd64, arm64)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("could not build fat target: %v\n%s\n", err, string(out))
	}

	// Run fat binary.
	cmd = exec.Command(fat)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("could not run fat target: %v", err)
	}
	if string(out) != "hello world\n" {
		t.Errorf("got=%s, want=hello world\n", string(out))
	}
}

// machoHeader returns a minimal little-endian 64-bit Mach-O header.
func machoHeader(cpu macho.Cpu, subcpu uint32) []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b[0:], macho.Magic64)
	binary.LittleEndian.PutUint32(b[4:], uint32(cpu))
	binary.LittleEndian.PutUint32(b[8:], subcpu)
	binary.LittleEndian.PutUint32(b[12:], uint32(macho.TypeExec))
	return b
}

func TestOutputIsInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows can't remove an open file")
	}
	dir, err := ioutil.TempDir("", "makefat")
	if err != nil {
		t.Fatalf("could not create directory: %v", err)
	}
	defer os.RemoveAll(dir)

	tool := filepath.Join(dir, "makefat")
	out, err := exec.Command("go", "build", "-o", tool, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("could not build makefat: %v\n%s\n", err, string(out))
	}

	amd64 := machoHeader(macho.CpuAmd64, 3)
	arm64 := machoHeader(macho.CpuArm64, 0)
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := ioutil.WriteFile(a, amd64, 0644); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(b, arm64, 0644); err != nil {
		t.Fatal(err)
	}

	// Write the fat file over one of its inputs. Kill makefat if the
	// output grows without bound, which is how this used to fail.
	cmd := exec.Command(tool, a, a, b)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for waiting := true; waiting; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("makefat failed: %v", err)
			}
			waiting = false
		case <-time.After(10 * time.Millisecond):
			if fi, err := os.Stat(a); err == nil && fi.Size() > 1<<20 {
				cmd.Process.Kill()
				<-done
				t.Fatalf("output grew to %d bytes", fi.Size())
			}
		}
	}

	f, err := macho.OpenFat(a)
	if err != nil {
		t.Fatalf("output is not a fat file: %v", err)
	}
	defer f.Close()
	want := [][]byte{amd64, arm64}
	if len(f.Arches) != len(want) {
		t.Fatalf("got %d arches, want %d", len(f.Arches), len(want))
	}
	data, err := ioutil.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	for i, arch := range f.Arches {
		got := data[arch.Offset : arch.Offset+arch.Size]
		if !bytes.Equal(got, want[i]) {
			t.Errorf("arch %d: contents differ from input", i)
		}
	}
}
