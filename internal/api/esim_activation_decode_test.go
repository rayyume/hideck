package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/yibaiba/hideck/internal/esim"
)

func TestHandleEsimDecodeActivationFromPNG(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := newEsimDisableTestServer(t)
	png, err := qrcode.Encode("LPA:1$smdp.example.com$API-TOKEN", qrcode.Medium, 256)
	if err != nil {
		t.Fatal(err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "qr.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "device_id", Value: "dev-esim"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/devices/dev-esim/esim/actions/decode-activation", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	server.handleEsimDecodeActivation(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var decoded esim.DecodedActivation
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SMDP != "smdp.example.com" || decoded.MatchingID != "API-TOKEN" {
		t.Fatalf("decoded=%#v", decoded)
	}
}

func TestHandleEsimDecodeActivationRequiresDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := newEsimDisableTestServer(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "device_id", Value: "missing"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/devices/missing/esim/actions/decode-activation", http.NoBody)
	server.handleEsimDecodeActivation(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", recorder.Code)
	}
}
