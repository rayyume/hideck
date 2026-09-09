//go:build darwin || linux

package pcsc

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

func TestNativePCSCLayout(test *testing.T) {
	wordSize := unsafe.Sizeof(uintptr(0))
	if runtime.GOOS == "darwin" {
		wordSize = 4
	}
	api := winscardAPI{}
	checks := map[string]uintptr{
		"DWORD output":      reflect.TypeOf(api.status).In(2).Elem().Size(),
		"LONG return":       reflect.TypeOf(api.status).Out(0).Size(),
		"context output":    reflect.TypeOf(api.establishContext).In(3).Elem().Size(),
		"card handle":       reflect.TypeOf(api.status).In(0).Size(),
		"PCI protocol":      unsafe.Sizeof(ioRequest{}.Protocol),
		"PCI length offset": unsafe.Offsetof(ioRequest{}.Length),
	}
	for name, actual := range checks {
		if actual != wordSize {
			test.Errorf("%s: got %d bytes, want %d", name, actual, wordSize)
		}
	}
	if actual := unsafe.Sizeof(ioRequest{}); actual != 2*wordSize {
		test.Errorf("PCI size: got %d bytes, want %d", actual, 2*wordSize)
	}
}

func TestNativePCSCErrorCodes(test *testing.T) {
	code := uint32(0x8010000C)
	for _, result := range []pcscLong{pcscLong(int32(code)), pcscLong(code)} {
		if err := checkPCSC("SCardConnect", result); !errors.Is(err, ErrNoCard) {
			test.Fatalf("result %d: expected ErrNoCard, got %v", result, err)
		}
	}
}

func TestNativePCSCCalls(test *testing.T) {
	compiler, err := exec.LookPath("cc")
	if err != nil {
		test.Skip("C compiler required for native ABI regression test")
	}
	libraryPath := filepath.Join(test.TempDir(), "pcsc-fixture.so")
	flags := []string{"-shared", "-fPIC"}
	if runtime.GOOS == "darwin" {
		flags = []string{"-dynamiclib"}
	}
	flags = append(flags, "-Wall", "-Wextra", "-Werror", "-o", libraryPath, "testdata/native_pcsc.c")
	if output, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		test.Fatalf("compile native fixture: %v\n%s", err, output)
	}
	library, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = purego.Dlclose(library) })
	var nativePCISize func() uintptr
	purego.RegisterLibFunc(&nativePCISize, library, "NativePCISize")
	if actual := nativePCISize(); actual != unsafe.Sizeof(ioRequest{}) {
		test.Fatalf("native PCI size %d differs from Go size %d", actual, unsafe.Sizeof(ioRequest{}))
	}
	api := &winscardAPI{library: library}
	if err := api.bind(); err != nil {
		test.Fatal(err)
	}
	backend := &systemBackend{api: api}
	backend.once.Do(func() {})
	readers, err := backend.Readers(context.Background())
	if err != nil {
		test.Fatal(err)
	}
	if len(readers) != 1 || readers[0].Name != "ABI Reader 00 00" || !readers[0].CardPresent || readers[0].ATR != "3B00" {
		test.Fatalf("unexpected readers: %+v", readers)
	}
	card, err := backend.Open(context.Background(), Selector{ReaderName: readers[0].Name})
	if err != nil {
		test.Fatal(err)
	}
	defer card.Close()
	response, status, err := card.Transmit(context.Background(), []byte{0x00, 0xA4, 0x00, 0x00})
	if err != nil || len(response) != 0 || status != 0x9000 {
		test.Fatalf("transmit: response=%X status=%04X err=%v", response, status, err)
	}
	if err := card.Close(); err != nil {
		test.Fatal(err)
	}
}
