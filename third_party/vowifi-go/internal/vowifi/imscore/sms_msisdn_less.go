package imscore

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/smscodec"
)

const (
	imsSMSXMLContentType        = "application/vnd.3gpp.sms+xml"
	imsSMSXMLDisposition        = `render; handling=optional`
	imsSMSFeatureCapsMSISDNless = "+g.3gpp.smsip-msisdn-less"
)

var errUnsupportedSMSContentType = errors.New("unsupported IMS SMS content type")

type shortMessageInfo struct {
	To   string
	From string
}

type smsDestination struct {
	display string
	tpDA    string
	sipURI  string
}

func (d smsDestination) msisdnLess() bool {
	return strings.TrimSpace(d.sipURI) != ""
}

type decodedIMSSMSPayload struct {
	rpdu []byte
	xml  shortMessageInfo
}

func parseSMSDestination(value string) (smsDestination, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return smsDestination{}, errors.New("imscore: SMS recipient is empty")
	}
	if strings.ContainsAny(value, "\r\n") {
		return smsDestination{}, errors.New("imscore: invalid SMS recipient")
	}
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "sip:"):
		return parseSIPSMSDestination(value)
	case strings.HasPrefix(lower, "tel:"):
		phone, err := normalizeSMSRecipient(strings.TrimSpace(value[4:]))
		if err != nil {
			return smsDestination{}, err
		}
		return smsDestination{display: phone, tpDA: phone}, nil
	default:
		phone, err := normalizeSMSRecipient(value)
		if err != nil {
			return smsDestination{}, err
		}
		return smsDestination{display: phone, tpDA: phone}, nil
	}
}

func parseSIPSMSDestination(uri string) (smsDestination, error) {
	rest := uri[4:]
	user, host, ok := strings.Cut(rest, "@")
	if !ok || strings.TrimSpace(user) == "" || strings.TrimSpace(host) == "" {
		return smsDestination{}, errors.New("imscore: invalid SMS SIP URI")
	}
	params := ""
	if _, remainder, found := strings.Cut(host, ";"); found {
		params = remainder
	}
	if strings.Contains(strings.ToLower(params), "user=phone") || sipUserLooksLikeMSISDN(user) {
		phone, err := normalizeSMSRecipient(user)
		if err != nil {
			return smsDestination{}, err
		}
		return smsDestination{display: phone, tpDA: phone}, nil
	}
	return smsDestination{display: uri, tpDA: smscodec.DummyMSISDN, sipURI: uri}, nil
}

func sipUserLooksLikeMSISDN(user string) bool {
	user = strings.TrimSpace(user)
	if user == "" {
		return false
	}
	_, err := normalizeSMSRecipient(user)
	return err == nil
}

func isSupportedSMSContentType(contentType string) bool {
	switch normalizedContentType(contentType) {
	case imsSMSContentType, "multipart/mixed":
		return true
	default:
		return false
	}
}

func extractIMSSMSPayload(contentType string, body []byte) (decodedIMSSMSPayload, error) {
	media, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		media = normalizedContentType(contentType)
	}
	switch strings.ToLower(strings.TrimSpace(media)) {
	case imsSMSContentType:
		rpdu, decodeErr := smscodec.DecodeBodyMaybeHex(body)
		if decodeErr != nil {
			return decodedIMSSMSPayload{}, decodeErr
		}
		return decodedIMSSMSPayload{rpdu: rpdu}, nil
	case "multipart/mixed":
		return extractMultipartIMSSMSPayload(params["boundary"], body)
	default:
		return decodedIMSSMSPayload{}, errUnsupportedSMSContentType
	}
}

func extractMultipartIMSSMSPayload(boundary string, body []byte) (decodedIMSSMSPayload, error) {
	if strings.TrimSpace(boundary) == "" {
		return decodedIMSSMSPayload{}, errors.New("IMS SMS multipart boundary is unavailable")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var payload decodedIMSSMSPayload
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return decodedIMSSMSPayload{}, err
		}
		partType := part.Header.Get("Content-Type")
		partBody, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			return decodedIMSSMSPayload{}, readErr
		}
		switch normalizedContentType(partType) {
		case imsSMSContentType:
			rpdu, decodeErr := smscodec.DecodeBodyMaybeHex(partBody)
			if decodeErr != nil {
				return decodedIMSSMSPayload{}, decodeErr
			}
			payload.rpdu = rpdu
		case imsSMSXMLContentType:
			info, parseErr := parseShortMessageInfoXML(partBody)
			if parseErr != nil {
				return decodedIMSSMSPayload{}, parseErr
			}
			payload.xml = info
		}
	}
	if len(payload.rpdu) == 0 {
		return decodedIMSSMSPayload{}, errors.New("IMS SMS multipart is missing application/vnd.3gpp.sms")
	}
	return payload, nil
}

func parseShortMessageInfoXML(body []byte) (shortMessageInfo, error) {
	var document struct {
		To   string `xml:"To"`
		From string `xml:"From"`
	}
	if err := xml.Unmarshal(body, &document); err != nil {
		return shortMessageInfo{}, err
	}
	return shortMessageInfo{
		To:   strings.TrimSpace(document.To),
		From: strings.TrimSpace(document.From),
	}, nil
}

func buildShortMessageInfoXML(info shortMessageInfo) []byte {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	builder.WriteString("<short-message-info>")
	if to := strings.TrimSpace(info.To); to != "" {
		builder.WriteString("<To>")
		_ = xml.EscapeText(&builder, []byte(to))
		builder.WriteString("</To>")
	}
	if from := strings.TrimSpace(info.From); from != "" {
		builder.WriteString("<From>")
		_ = xml.EscapeText(&builder, []byte(from))
		builder.WriteString("</From>")
	}
	builder.WriteString("</short-message-info>")
	return []byte(builder.String())
}

func buildMSISDNLessSMSPayload(info shortMessageInfo, rpdu []byte) (string, []byte, error) {
	if len(rpdu) == 0 {
		return "", nil, errors.New("IMS MSISDN-less SMS payload is empty")
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	xmlHeader := textproto.MIMEHeader{}
	xmlHeader.Set("Content-Type", imsSMSXMLContentType)
	xmlHeader.Set("Content-Disposition", imsSMSXMLDisposition)
	xmlPart, err := writer.CreatePart(xmlHeader)
	if err != nil {
		return "", nil, err
	}
	if _, err = xmlPart.Write(buildShortMessageInfoXML(info)); err != nil {
		return "", nil, err
	}
	smsHeader := textproto.MIMEHeader{}
	smsHeader.Set("Content-Type", imsSMSContentType)
	smsHeader.Set("Content-Transfer-Encoding", "binary")
	smsPart, err := writer.CreatePart(smsHeader)
	if err != nil {
		return "", nil, err
	}
	if _, err = smsPart.Write(rpdu); err != nil {
		return "", nil, err
	}
	if err = writer.Close(); err != nil {
		return "", nil, err
	}
	return "multipart/mixed;boundary=" + writer.Boundary(), buffer.Bytes(), nil
}

func hasMSISDNLessFeatureCaps(raw string) bool {
	value := strings.ToLower(rawSIPHeaderValue(raw, "Feature-Caps"))
	return strings.Contains(value, imsSMSFeatureCapsMSISDNless) ||
		strings.Contains(value, "+g.3gpp.smsip-msisdnless")
}
