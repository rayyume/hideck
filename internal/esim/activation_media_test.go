package esim

import (
	"bytes"
	"io"
	"testing"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	qrcode "github.com/skip2/go-qrcode"
)

const testActivation = "LPA:1$smdp.example.com$MARKET-TOKEN"

func testActivationPNG(t *testing.T) []byte {
	t.Helper()
	png, err := qrcode.Encode(testActivation, qrcode.Medium, 256)
	if err != nil {
		t.Fatal(err)
	}
	return png
}

func TestDecodeActivationMediaFromPNG(t *testing.T) {
	decoded, err := DecodeActivationMedia(testActivationPNG(t), "qr.png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Text != testActivation || decoded.SMDP != "smdp.example.com" || decoded.MatchingID != "MARKET-TOKEN" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestDecodeActivationMediaFromPDF(t *testing.T) {
	var pdf bytes.Buffer
	if err := pdfcpuapi.ImportImages(nil, &pdf, []io.Reader{bytes.NewReader(testActivationPNG(t))}, nil, nil); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeActivationMedia(pdf.Bytes(), "voxi.pdf", "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.MatchingID != "MARKET-TOKEN" {
		t.Fatalf("pdf decoded = %#v", decoded)
	}
}

func TestDecodeActivationMediaRejectsEmptyAndUnknown(t *testing.T) {
	if _, err := DecodeActivationMedia(nil, "qr.png", "image/png"); err == nil {
		t.Fatal("empty media should fail")
	}
	if _, err := DecodeActivationMedia([]byte("not a qr"), "note.txt", "text/plain"); err == nil {
		t.Fatal("unknown media should fail")
	}
}
