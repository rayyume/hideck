package phonelookup

import "strings"

var regionByMCC = map[string]string{
	"204": "NL",
	"208": "FR",
	"228": "CH",
	"234": "GB",
	"235": "GB",
	"248": "EE",
	"262": "DE",
	"302": "CA",
	"310": "US",
	"311": "US",
	"312": "US",
	"313": "US",
	"314": "US",
	"315": "US",
	"316": "US",
	"317": "US",
	"318": "US",
	"440": "JP",
	"454": "HK",
	"460": "CN",
	"502": "MY",
	"515": "PH",
	"520": "TH",
	"530": "NZ",
	"621": "NG",
}

func RegionForMCC(mcc string) string {
	return regionByMCC[strings.TrimSpace(mcc)]
}
