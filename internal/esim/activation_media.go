package esim

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/makiuchi-d/gozxing"
	zxingqr "github.com/makiuchi-d/gozxing/qrcode"
	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"golang.org/x/image/webp"
)

const maxActivationMediaBytes = 12 << 20

// DecodedActivation is the QR payload extracted from an image or PDF.
type DecodedActivation struct {
	Text                 string `json:"text"`
	SMDP                 string `json:"smdp,omitempty"`
	MatchingID           string `json:"matching_id,omitempty"`
	ConfirmationRequired bool   `json:"confirmation_required"`
	ConfirmationCode     string `json:"confirmation_code,omitempty"`
}

// DecodeActivationMedia reads a QR from a screenshot, photo, or carrier PDF.
func DecodeActivationMedia(data []byte, filename, contentType string) (DecodedActivation, error) {
	if len(data) == 0 {
		return DecodedActivation{}, fmt.Errorf("请上传二维码图片或 PDF")
	}
	if len(data) > maxActivationMediaBytes {
		return DecodedActivation{}, fmt.Errorf("文件太大，请换一张截图或更小的 PDF")
	}
	var texts []string
	if looksLikePDF(data, filename, contentType) {
		extracted, err := decodeQRCodesFromPDF(data)
		if err != nil {
			return DecodedActivation{}, err
		}
		texts = extracted
	} else {
		text, err := decodeQRCodeFromImageBytes(data)
		if err != nil {
			return DecodedActivation{}, err
		}
		if text != "" {
			texts = []string{text}
		}
	}
	if len(texts) == 0 {
		return DecodedActivation{}, fmt.Errorf("没有识别到二维码，请换一张更清晰的截图或带二维码的 PDF")
	}
	chosen := pickActivationText(texts)
	decoded := DecodedActivation{Text: chosen}
	if parsed, err := ParseActivationCode(chosen); err == nil {
		decoded.SMDP = parsed.SMDP
		decoded.MatchingID = parsed.MatchingID
		decoded.ConfirmationRequired = parsed.ConfirmationRequired
		decoded.ConfirmationCode = parsed.ConfirmationCode
	}
	return decoded, nil
}

func pickActivationText(texts []string) string {
	for _, text := range texts {
		if _, err := ParseActivationCode(text); err == nil {
			return text
		}
	}
	return texts[0]
}

func looksLikePDF(data []byte, filename, contentType string) bool {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("%PDF")) {
		return true
	}
	if strings.Contains(strings.ToLower(contentType), "pdf") {
		return true
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(filename)), ".pdf")
}

func decodeQRCodesFromPDF(data []byte) ([]string, error) {
	conf := model.NewDefaultConfiguration()
	var texts []string
	err := pdfcpuapi.ExtractImages(bytes.NewReader(data), nil, func(img model.Image, _ bool, _ int) error {
		payload, readErr := io.ReadAll(io.LimitReader(img, maxActivationMediaBytes+1))
		if readErr != nil || len(payload) == 0 || len(payload) > maxActivationMediaBytes {
			return nil
		}
		text, decodeErr := decodeQRCodeFromImageBytes(payload)
		if decodeErr == nil && text != "" {
			texts = append(texts, text)
		}
		return nil
	}, conf)
	if err != nil && len(texts) == 0 {
		return nil, fmt.Errorf("无法读取 PDF，请改用二维码截图")
	}
	return texts, nil
}

func decodeQRCodeFromImageBytes(data []byte) (string, error) {
	img, err := decodeRasterImage(data)
	if err != nil {
		return "", fmt.Errorf("无法读取图片，请换 PNG、JPEG 或 PDF")
	}
	if text := decodeQRImage(img); text != "" {
		return text, nil
	}
	return "", nil
}

func decodeRasterImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return img, nil
	}
	webpImg, webpErr := webp.Decode(bytes.NewReader(data))
	if webpErr == nil {
		return webpImg, nil
	}
	return nil, err
}

func decodeQRImage(img image.Image) string {
	if text := readQRBitmap(img); text != "" {
		return text
	}
	return readQRBitmap(invertImage(img))
}

func readQRBitmap(img image.Image) string {
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return ""
	}
	result, err := zxingqr.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER:       true,
		gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{gozxing.BarcodeFormat_QR_CODE},
	})
	if err != nil || result == nil {
		return ""
	}
	return strings.TrimSpace(result.GetText())
}

func invertImage(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			dst.SetRGBA(x, y, color.RGBA{R: 255 - uint8(r>>8), G: 255 - uint8(g>>8), B: 255 - uint8(b>>8), A: uint8(a >> 8)})
		}
	}
	return dst
}
