package volte

import "testing"

func TestNewALSAHWParamsLeavesIntervalsOpen(t *testing.T) {
	params := newALSAHWParams()
	min, max, flags := alsaIntervalValue(params, alsaParamRate)
	if min != 0 || max != 0xffffffff || flags != 0 {
		t.Fatalf("any rate interval min=%d max=%d flags=%d", min, max, flags)
	}
}

func TestConstrainALSAIntervalSetsIntegerNotOpenMin(t *testing.T) {
	params := newALSAHWParams()
	constrainALSAInterval(&params, alsaParamRate, 8000)
	min, max, flags := alsaIntervalValue(params, alsaParamRate)
	if min != 8000 || max != 8000 {
		t.Fatalf("rate min=%d max=%d", min, max)
	}
	if flags&alsaIntervalInt == 0 {
		t.Fatal("integer flag missing")
	}
	if flags&1 != 0 {
		t.Fatal("openmin must stay 0 when min==max, otherwise HW_PARAMS is empty and returns EINVAL")
	}
}

func TestConstrainALSAVoicePCM(t *testing.T) {
	params := newALSAHWParams()
	constrainALSAMask(&params, alsaParamAccess, alsaAccessRWInter)
	constrainALSAMask(&params, alsaParamFormat, alsaFormatS16LE)
	constrainALSAInterval(&params, alsaParamChannels, 1)
	constrainALSAInterval(&params, alsaParamRate, uint32(pcmuClockRate))
	min, max, flags := alsaIntervalValue(params, alsaParamChannels)
	if min != 1 || max != 1 || flags != alsaIntervalInt {
		t.Fatalf("channels min=%d max=%d flags=%d", min, max, flags)
	}
	min, max, flags = alsaIntervalValue(params, alsaParamRate)
	if min != 8000 || max != 8000 || flags != alsaIntervalInt {
		t.Fatalf("rate min=%d max=%d flags=%d", min, max, flags)
	}
}
