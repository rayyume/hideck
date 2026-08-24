package volte

import "testing"

func TestParseCOPSAndCEREG(t *testing.T) {
	mcc, mnc, err := ParseCOPS(`+COPS: 0,2,"46000",7`)
	if err != nil || mcc != "460" || mnc != "00" {
		t.Fatalf("cops %s/%s err=%v", mcc, mnc, err)
	}
	lte, err := ParseCEREG("+CEREG: 0,1\r\nOK")
	if err != nil || !lte.Registered {
		t.Fatalf("cereg %+v err=%v", lte, err)
	}
	denied, err := ParseCEREG("+CEREG: 2,3")
	if err != nil || denied.Registered || denied.Stat != 3 {
		t.Fatalf("denied %+v err=%v", denied, err)
	}
}

func TestParseIMSContext(t *testing.T) {
	ctxs, err := ParseCGDCONT(`+CGDCONT: 1,"IP","cmnet","0.0.0.0",0,0
+CGDCONT: 2,"IPV4V6","ims","0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0",0,0
OK`)
	if err != nil {
		t.Fatal(err)
	}
	ims, ok := IMSContext(ctxs)
	if !ok || ims.CID != 2 {
		t.Fatalf("ims %+v ok=%v", ims, ok)
	}
	act, err := ParseCGACT("+CGACT: 1,1\r\n+CGACT: 2,0\r\nOK")
	if err != nil || act[2] {
		t.Fatalf("cgact %v err=%v", act, err)
	}
	if CGACTSetCommand(2, true) != "AT+CGACT=1,2" {
		t.Fatal(CGACTSetCommand(2, true))
	}
}
