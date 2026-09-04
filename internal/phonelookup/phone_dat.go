package phonelookup

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"io"
	"strconv"
	"strings"
	"sync"
)

// 国内手机号段：pangongzi/phone 的 phone.dat（2025-02，约 51 万条 7 位号段）。
//
//go:embed data/phone.dat.gz
var phoneDatGZ []byte

type cnHLRRecord struct {
	Province string
	City     string
	ZipCode  string
	AreaZone string
	CardType string
}

const (
	phoneDatIndexLen = 9
)

var phoneDatCardTypes = map[byte]string{
	1: "中国移动",
	2: "中国联通",
	3: "中国电信",
	4: "中国电信",
	5: "中国联通",
	6: "中国移动",
	7: "中国广电",
	8: "中国广电",
}

var (
	phoneDatOnce   sync.Once
	phoneDat       []byte
	phoneDatOffset int32
)

func loadPhoneDat() {
	phoneDatOnce.Do(func() {
		r, err := gzip.NewReader(bytes.NewReader(phoneDatGZ))
		if err != nil {
			return
		}
		defer r.Close()
		raw, err := io.ReadAll(r)
		if err != nil || len(raw) < 8 {
			return
		}
		off := int32(binary.LittleEndian.Uint32(raw[4:8]))
		if off < 8 || int(off) >= len(raw) {
			return
		}
		phoneDat = raw
		phoneDatOffset = off
	})
}

func lookupCNHLR(national string) (cnHLRRecord, bool) {
	loadPhoneDat()
	if len(phoneDat) < 8 || len(national) < 7 {
		return cnHLRRecord{}, false
	}
	n64, err := strconv.ParseInt(national[:7], 10, 32)
	if err != nil {
		return cnHLRRecord{}, false
	}
	want := int32(n64)
	count := (int32(len(phoneDat)) - phoneDatOffset) / phoneDatIndexLen
	if count <= 0 {
		return cnHLRRecord{}, false
	}
	left, right := int32(0), count-1
	for left <= right {
		mid := left + (right-left)/2
		off := phoneDatOffset + mid*phoneDatIndexLen
		if off < 0 || int(off)+phoneDatIndexLen > len(phoneDat) {
			return cnHLRRecord{}, false
		}
		cur := int32(binary.LittleEndian.Uint32(phoneDat[off : off+4]))
		switch {
		case cur > want:
			right = mid - 1
		case cur < want:
			left = mid + 1
		default:
			recOff := int32(binary.LittleEndian.Uint32(phoneDat[off+4 : off+8]))
			card := phoneDat[off+8]
			return parseCNHLRRecord(recOff, card)
		}
	}
	return cnHLRRecord{}, false
}

func parseCNHLRRecord(recOff int32, card byte) (cnHLRRecord, bool) {
	if recOff < 0 || int(recOff) >= len(phoneDat) {
		return cnHLRRecord{}, false
	}
	rest := phoneDat[recOff:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return cnHLRRecord{}, false
	}
	parts := bytes.Split(rest[:end], []byte("|"))
	if len(parts) < 2 {
		return cnHLRRecord{}, false
	}
	rec := cnHLRRecord{
		Province: string(parts[0]),
		City:     string(parts[1]),
		CardType: phoneDatCardTypes[card],
	}
	if rec.CardType == "" {
		rec.CardType = "未知运营商"
	}
	if len(parts) > 2 {
		rec.ZipCode = string(parts[2])
	}
	if len(parts) > 3 {
		rec.AreaZone = string(parts[3])
	}
	return rec, true
}

func formatCNRegion(province, city string) string {
	province = strings.TrimSpace(province)
	city = strings.TrimSpace(city)
	switch {
	case province == "" && city == "":
		return ""
	case city == "" || city == province:
		return province
	case strings.Contains(city, province):
		return city
	default:
		return province + city
	}
}
