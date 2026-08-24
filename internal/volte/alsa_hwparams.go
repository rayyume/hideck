package volte

import "encoding/binary"

const (
	alsaHWParamsSize  = 608
	alsaParamAccess   = 0
	alsaParamFormat   = 1
	alsaParamChannels = 10
	alsaParamRate     = 11
	alsaParamPeriodSz = 13
	alsaParamBufferSz = 17
	alsaAccessRWInter = 3
	alsaFormatS16LE   = 2
	alsaIntervalFirst = 8
	alsaIntervalCount = 21
	alsaRmaskOff      = 512
	alsaIntervalInt   = 1 << 2
)

func newALSAHWParams() [alsaHWParamsSize]byte {
	var params [alsaHWParamsSize]byte
	for i := 4; i < 4+8*32; i++ {
		params[i] = 0xff
	}
	for i := 0; i < alsaIntervalCount; i++ {
		off := 260 + i*12
		binary.LittleEndian.PutUint32(params[off+4:], 0xffffffff)
	}
	binary.LittleEndian.PutUint32(params[alsaRmaskOff:], 0xffffffff)
	return params
}

func constrainALSAMask(params *[alsaHWParamsSize]byte, param, bit int) {
	if param < 0 || param > 2 {
		return
	}
	off := 4 + param*32
	for i := 0; i < 32; i++ {
		params[off+i] = 0
	}
	word := bit / 32
	shift := uint(bit % 32)
	if word < 0 || word >= 8 {
		return
	}
	binary.LittleEndian.PutUint32(params[off+word*4:], uint32(1)<<shift)
}

func constrainALSAInterval(params *[alsaHWParamsSize]byte, param int, value uint32) {
	idx := param - alsaIntervalFirst
	if idx < 0 || idx >= 12 {
		return
	}
	off := 260 + idx*12
	binary.LittleEndian.PutUint32(params[off:], value)
	binary.LittleEndian.PutUint32(params[off+4:], value)
	binary.LittleEndian.PutUint32(params[off+8:], alsaIntervalInt)
}

func alsaIntervalValue(params [alsaHWParamsSize]byte, param int) (min, max, flags uint32) {
	idx := param - alsaIntervalFirst
	if idx < 0 || idx >= 12 {
		return 0, 0, 0
	}
	off := 260 + idx*12
	return binary.LittleEndian.Uint32(params[off:]),
		binary.LittleEndian.Uint32(params[off+4:]),
		binary.LittleEndian.Uint32(params[off+8:])
}
