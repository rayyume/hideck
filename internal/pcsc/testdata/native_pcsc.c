#include <stdint.h>
#include <stddef.h>
#include <string.h>

#ifdef __APPLE__
typedef uint32_t DWORD;
typedef int32_t LONG;
#else
typedef unsigned long DWORD;
typedef long LONG;
#endif

typedef struct {
    DWORD protocol;
    DWORD length;
} PCI;

static const char reader_name[] = "ABI Reader 00 00";
static const LONG context_handle = 0x12345678;
static const LONG card_handle = 0x23456789;

size_t NativePCISize(void) { return sizeof(PCI); }

LONG SCardEstablishContext(DWORD scope, const void *reserved_first,
                          const void *reserved_second, LONG *context) {
    if (scope != 2 || reserved_first || reserved_second) return -1;
    *context = context_handle;
    return 0;
}

LONG SCardReleaseContext(LONG context) {
    return context == context_handle ? 0 : -1;
}

LONG SCardListReaders(LONG context, const char *groups, char *readers, DWORD *length) {
    if (context != context_handle || groups) return -1;
    DWORD required = sizeof(reader_name) + 1;
    if (readers) {
        if (*length != required) return -1;
        memcpy(readers, reader_name, sizeof(reader_name));
        readers[sizeof(reader_name)] = '\0';
    }
    *length = required;
    return 0;
}

LONG SCardConnect(LONG context, const char *reader, DWORD sharing,
                  DWORD protocols, LONG *card, DWORD *protocol) {
    if (context != context_handle || strcmp(reader, reader_name) || sharing != 2 || protocols != 3) return -1;
    *card = card_handle;
    *protocol = 1;
    return 0;
}

LONG SCardDisconnect(LONG card, DWORD disposition) {
    return card == card_handle && disposition == 0 ? 0 : -1;
}

LONG SCardBeginTransaction(LONG card) {
    return card == card_handle ? 0 : -1;
}

LONG SCardEndTransaction(LONG card, DWORD disposition) {
    return SCardDisconnect(card, disposition);
}

LONG SCardStatus(LONG card, char *reader, DWORD *reader_length, DWORD *state,
                 DWORD *protocol, unsigned char *atr, DWORD *atr_length) {
    if (card != card_handle || *reader_length != 256 || *atr_length != 64) return -1;
    memcpy(reader, reader_name, sizeof(reader_name));
    *reader_length = sizeof(reader_name);
    *state = 6;
    *protocol = 1;
    atr[0] = 0x3B;
    atr[1] = 0x00;
    *atr_length = 2;
    return 0;
}

LONG SCardGetAttrib(LONG card, DWORD attribute, unsigned char *value, DWORD *length) {
    if (card != card_handle || attribute != 0x00020110 || *length != 8) return -1;
    memset(value, 0, 4);
    *length = 4;
    return 0;
}

LONG SCardTransmit(LONG card, const PCI *send_pci, const unsigned char *command,
                   DWORD command_length, PCI *receive_pci, unsigned char *response,
                   DWORD *response_length) {
    if (card != card_handle || send_pci->protocol != 1 || send_pci->length != sizeof(PCI) ||
        command_length != 4 || command[1] != 0xA4 || receive_pci || *response_length != 65546) return -1;
    response[0] = 0x90;
    response[1] = 0x00;
    *response_length = 2;
    return 0;
}
