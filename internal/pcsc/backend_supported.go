//go:build darwin || linux

package pcsc

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

const (
	pcscScopeSystem   = 2
	pcscShareShared   = 2
	pcscShareDirect   = 3
	pcscProtocolAny   = 3
	pcscLeaveCard     = 0
	pcscResetCard     = 1
	pcscAttrChannelID = 0x00020110
)

type systemBackend struct {
	once sync.Once
	api  *winscardAPI
	err  error
}

type winscardAPI struct {
	library          uintptr
	establishContext func(pcscDword, uintptr, uintptr, *pcscLong) pcscLong
	releaseContext   func(pcscLong) pcscLong
	listReaders      func(pcscLong, uintptr, []byte, *pcscDword) pcscLong
	connect          func(pcscLong, string, pcscDword, pcscDword, *pcscLong, *pcscDword) pcscLong
	disconnect       func(pcscLong, pcscDword) pcscLong
	beginTransaction func(pcscLong) pcscLong
	endTransaction   func(pcscLong, pcscDword) pcscLong
	status           func(pcscLong, []byte, *pcscDword, *pcscDword, *pcscDword, []byte, *pcscDword) pcscLong
	transmit         func(pcscLong, *ioRequest, []byte, pcscDword, *ioRequest, []byte, *pcscDword) pcscLong
	getAttrib        func(pcscLong, pcscDword, []byte, *pcscDword) pcscLong
}

func newSystemBackend() Backend { return &systemBackend{} }

func (backend *systemBackend) load() (*winscardAPI, error) {
	backend.once.Do(func() { backend.api, backend.err = loadWinSCard() })
	return backend.api, backend.err
}

func loadWinSCard() (*winscardAPI, error) {
	var candidates []string
	if runtime.GOOS == "darwin" {
		candidates = []string{"/System/Library/Frameworks/PCSC.framework/PCSC"}
	} else {
		candidates = []string{"libpcsclite.so.1", "libpcsclite.so"}
	}
	var failures []error
	for _, candidate := range candidates {
		handle, err := purego.Dlopen(candidate, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		api := &winscardAPI{library: handle}
		if err := api.bind(); err != nil {
			_ = purego.Dlclose(handle)
			failures = append(failures, err)
			continue
		}
		return api, nil
	}
	return nil, fmt.Errorf("%w: load system PC/SC library: %w", ErrUnavailable, errors.Join(failures...))
}

func registerSymbol(handle uintptr, target any, name string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("pcsc: resolve %s: %v", name, recovered)
		}
	}()
	purego.RegisterLibFunc(target, handle, name)
	return nil
}

func (api *winscardAPI) bind() error {
	bindings := []struct {
		target any
		name   string
	}{
		{&api.establishContext, "SCardEstablishContext"}, {&api.releaseContext, "SCardReleaseContext"},
		{&api.listReaders, "SCardListReaders"}, {&api.connect, "SCardConnect"},
		{&api.disconnect, "SCardDisconnect"}, {&api.beginTransaction, "SCardBeginTransaction"},
		{&api.endTransaction, "SCardEndTransaction"}, {&api.status, "SCardStatus"},
		{&api.transmit, "SCardTransmit"}, {&api.getAttrib, "SCardGetAttrib"},
	}
	for _, binding := range bindings {
		if err := registerSymbol(api.library, binding.target, binding.name); err != nil {
			return err
		}
	}
	return nil
}

type pcscError struct {
	Operation string
	Code      uint32
}

func (err *pcscError) Error() string {
	return fmt.Sprintf("pcsc: %s failed with code 0x%08X", err.Operation, err.Code)
}

func checkPCSC(operation string, result pcscLong) error {
	if result == 0 {
		return nil
	}
	code := uint32(result)
	switch code {
	case 0x8010001D, 0x8010001E:
		return fmt.Errorf("%w: %s", ErrUnavailable, (&pcscError{operation, code}).Error())
	case 0x80100009:
		return fmt.Errorf("%w: %s", ErrReaderNotFound, (&pcscError{operation, code}).Error())
	case 0x8010000C, 0x80100069:
		return fmt.Errorf("%w: %s", ErrNoCard, (&pcscError{operation, code}).Error())
	default:
		return &pcscError{Operation: operation, Code: code}
	}
}

func (api *winscardAPI) context() (pcscLong, error) {
	var handle pcscLong
	if err := checkPCSC("SCardEstablishContext", api.establishContext(pcscScopeSystem, 0, 0, &handle)); err != nil {
		return 0, err
	}
	return handle, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (backend *systemBackend) Readers(ctx context.Context) ([]Reader, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	api, err := backend.load()
	if err != nil {
		return nil, err
	}
	handle, err := api.context()
	if err != nil {
		return nil, err
	}
	defer api.releaseContext(handle)
	names, err := api.readerNames(handle)
	if err != nil {
		return nil, err
	}
	readers := make([]Reader, 0, len(names))
	for _, name := range names {
		reader := Reader{Name: name, Product: readerProduct(name), USBPath: "pcsc:" + name}
		api.enrichReader(handle, &reader)
		readers = append(readers, reader)
	}
	return readers, contextError(ctx)
}

func (api *winscardAPI) readerNames(handle pcscLong) ([]string, error) {
	var size pcscDword
	result := api.listReaders(handle, 0, nil, &size)
	if uint32(result) == 0x8010002E {
		return []string{}, nil
	}
	if err := checkPCSC("SCardListReaders", result); err != nil {
		return nil, err
	}
	if size == 0 {
		return []string{}, nil
	}
	buffer := make([]byte, size)
	if err := checkPCSC("SCardListReaders", api.listReaders(handle, 0, buffer, &size)); err != nil {
		return nil, err
	}
	return decodeMultiString(buffer[:min(int(size), len(buffer))]), nil
}

func decodeMultiString(buffer []byte) []string {
	var result []string
	for len(buffer) > 0 {
		end := 0
		for end < len(buffer) && buffer[end] != 0 {
			end++
		}
		if end == 0 {
			break
		}
		result = append(result, string(buffer[:end]))
		buffer = buffer[min(end+1, len(buffer)):]
	}
	return result
}

func readerProduct(name string) string {
	trimmed := strings.TrimSpace(name)
	for _, suffix := range []string{" 00 00", " 00 01"} {
		trimmed = strings.TrimSuffix(trimmed, suffix)
	}
	return strings.TrimSpace(trimmed)
}

func (api *winscardAPI) enrichReader(contextHandle pcscLong, reader *Reader) {
	var card pcscLong
	var protocol pcscDword
	result := api.connect(contextHandle, reader.Name+"\x00", pcscShareShared, pcscProtocolAny, &card, &protocol)
	if result == 0 {
		reader.CardPresent = true
		api.readCardDetails(card, reader)
		_ = api.disconnect(card, pcscLeaveCard)
		return
	}
	result = api.connect(contextHandle, reader.Name+"\x00", pcscShareDirect, 0, &card, &protocol)
	if result == 0 {
		api.readUSBPath(card, reader)
		_ = api.disconnect(card, pcscLeaveCard)
	}
}

func (api *winscardAPI) readCardDetails(card pcscLong, reader *Reader) {
	nameBuffer := make([]byte, 256)
	atrBuffer := make([]byte, 64)
	nameLength, atrLength := pcscDword(len(nameBuffer)), pcscDword(len(atrBuffer))
	var state, protocol pcscDword
	result := api.status(card, nameBuffer, &nameLength, &state, &protocol, atrBuffer, &atrLength)
	if result == 0 && atrLength <= pcscDword(len(atrBuffer)) {
		reader.ATR = strings.ToUpper(hex.EncodeToString(atrBuffer[:atrLength]))
	}
	api.readUSBPath(card, reader)
}

func (api *winscardAPI) readUSBPath(card pcscLong, reader *Reader) {
	attribute := make([]byte, 8)
	length := pcscDword(len(attribute))
	if api.getAttrib(card, pcscAttrChannelID, attribute, &length) != 0 || length < 4 {
		return
	}
	channel := binary.LittleEndian.Uint32(attribute[:4])
	if path, ok := systemUSBPath(channel); ok {
		reader.USBPath = path
		enrichUSBReader(reader)
	}
}

func (backend *systemBackend) Open(ctx context.Context, selector Selector) (Card, error) {
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	readers, err := backend.Readers(ctx)
	if err != nil {
		return nil, err
	}
	reader, ok := MatchReader(readers, selector)
	if !ok {
		return nil, ErrReaderNotFound
	}
	if !reader.CardPresent {
		return nil, ErrNoCard
	}
	api, err := backend.load()
	if err != nil {
		return nil, err
	}
	contextHandle, err := api.context()
	if err != nil {
		return nil, err
	}
	var cardHandle pcscLong
	var protocol pcscDword
	result := api.connect(contextHandle, reader.Name+"\x00", pcscShareShared, pcscProtocolAny, &cardHandle, &protocol)
	if err := checkPCSC("SCardConnect", result); err != nil {
		_ = api.releaseContext(contextHandle)
		return nil, err
	}
	if err := checkPCSC("SCardBeginTransaction", api.beginTransaction(cardHandle)); err != nil {
		_ = api.disconnect(cardHandle, pcscLeaveCard)
		_ = api.releaseContext(contextHandle)
		return nil, err
	}
	return &systemCard{api: api, context: contextHandle, handle: cardHandle, protocol: protocol}, nil
}
