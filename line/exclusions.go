package line

import "fmt"

// Exclusion records an out-of-subset AFP construct encountered during
// reduction. Exclusions are loud by design: reduction continues, but every
// excluded construct is reported with its field type and reason. Nothing is
// silently dropped.
type Exclusion struct {
	// FieldType names the excluded construct, e.g. "GOCA" or a field
	// acronym such as "BGR".
	FieldType string
	// Reason explains why it is outside the v1 contracted subset.
	Reason string
}

// Diagnostic renders the exclusion as a human-readable message naming the
// field type and the reason.
func (e Exclusion) Diagnostic() string {
	return fmt.Sprintf("excluded %s: %s", e.FieldType, e.Reason)
}

// closedExclusions is the closed exclusion list from the subset contract §9.
// Keyed by field acronym; the reason names the owning architecture.
var closedExclusions = map[string]string{
	// IOCA (classified excluded per contract §9)
	"BIM": "IOCA image objects are outside the v1 contracted subset",
	"EIM": "IOCA image objects are outside the v1 contracted subset",
	"IPD": "IOCA image objects are outside the v1 contracted subset",
	"IDD": "IOCA image objects are outside the v1 contracted subset",
	"IID": "IOCA image objects are outside the v1 contracted subset",
	"IOC": "IOCA image objects are outside the v1 contracted subset",
	"ICP": "IOCA image objects are outside the v1 contracted subset",
	"IRD": "IOCA image objects are outside the v1 contracted subset",
	// GOCA
	"BGR": "GOCA vector graphics are outside the v1 contracted subset",
	"EGR": "GOCA vector graphics are outside the v1 contracted subset",
	"GAD": "GOCA vector graphics are outside the v1 contracted subset",
	"GDD": "GOCA vector graphics are outside the v1 contracted subset",
	// BCOCA
	"BBC": "BCOCA barcodes are outside the v1 contracted subset",
	"EBC": "BCOCA barcodes are outside the v1 contracted subset",
	"BDA": "BCOCA barcodes are outside the v1 contracted subset",
	"BDD": "BCOCA barcodes are outside the v1 contracted subset",
	"MBC": "BCOCA barcodes are outside the v1 contracted subset",
	// FOCA
	"BCF": "FOCA font resources are outside the v1 contracted subset",
	"ECF": "FOCA font resources are outside the v1 contracted subset",
	"BFN": "FOCA font resources are outside the v1 contracted subset",
	"EFN": "FOCA font resources are outside the v1 contracted subset",
	"BCP": "FOCA font resources are outside the v1 contracted subset",
	"ECP": "FOCA font resources are outside the v1 contracted subset",
	"CFC": "FOCA font resources are outside the v1 contracted subset",
	"CFI": "FOCA font resources are outside the v1 contracted subset",
	"CPC": "FOCA font resources are outside the v1 contracted subset",
	"CPD": "FOCA font resources are outside the v1 contracted subset",
	"CPI": "FOCA font resources are outside the v1 contracted subset",
	"FNC": "FOCA font resources are outside the v1 contracted subset",
	"FND": "FOCA font resources are outside the v1 contracted subset",
	"FNG": "FOCA font resources are outside the v1 contracted subset",
	"FNI": "FOCA font resources are outside the v1 contracted subset",
	"FNM": "FOCA font resources are outside the v1 contracted subset",
	"FNN": "FOCA font resources are outside the v1 contracted subset",
	"FNO": "FOCA font resources are outside the v1 contracted subset",
	"FNP": "FOCA font resources are outside the v1 contracted subset",
	// CMOCA / color
	"BCA": "CMOCA color management is outside the v1 contracted subset",
	"ECA": "CMOCA color management is outside the v1 contracted subset",
	"MCA": "CMOCA color management is outside the v1 contracted subset",
	"CAT": "CMOCA color management is outside the v1 contracted subset",
	// Overlays / medium
	"BMO": "overlays and medium maps are outside the v1 contracted subset",
	"EMO": "overlays and medium maps are outside the v1 contracted subset",
	"BMM": "overlays and medium maps are outside the v1 contracted subset",
	"EMM": "overlays and medium maps are outside the v1 contracted subset",
	"MMO": "overlays and medium maps are outside the v1 contracted subset",
	"MPO": "overlays and medium maps are outside the v1 contracted subset",
	"IMM": "overlays and medium maps are outside the v1 contracted subset",
	"MCC": "overlays and medium maps are outside the v1 contracted subset",
	"MDD": "overlays and medium maps are outside the v1 contracted subset",
	"MMC": "overlays and medium maps are outside the v1 contracted subset",
	"BRG": "overlays and medium maps are outside the v1 contracted subset",
	"ERG": "overlays and medium maps are outside the v1 contracted subset",
	"BSG": "overlays and medium maps are outside the v1 contracted subset",
	"ESG": "overlays and medium maps are outside the v1 contracted subset",
	"BDG": "overlays and medium maps are outside the v1 contracted subset",
	"EDG": "overlays and medium maps are outside the v1 contracted subset",
	"BOG": "overlays and medium maps are outside the v1 contracted subset",
	"EOG": "overlays and medium maps are outside the v1 contracted subset",
	// FormDef / PageDef carriers (per contract §7)
	"BFM": "FormDef/PageDef medium definitions are outside the v1 contracted subset",
	"EFM": "FormDef/PageDef medium definitions are outside the v1 contracted subset",
	"BPM": "FormDef/PageDef medium definitions are outside the v1 contracted subset",
	"EPM": "FormDef/PageDef medium definitions are outside the v1 contracted subset",
	// Object containers
	"BOC": "object containers are outside the v1 contracted subset",
	"EOC": "object containers are outside the v1 contracted subset",
	"OCD": "object containers are outside the v1 contracted subset",
	// Misc skip
	"NOP": "no-operation fields carry no content",
	"TLE": "tag logical elements carry indexing metadata, not presentation content",
	"IPS": "index structures are outside the v1 contracted subset",
	"MPS": "index structures are outside the v1 contracted subset",
}

// ClassifyExclusion looks up a field acronym in the closed exclusion list.
// It returns the Exclusion and true when the acronym is a known exclusion;
// false means the acronym is not an exclusion (it may be in-subset or unknown).
func ClassifyExclusion(acronym string) (Exclusion, bool) {
	reason, ok := closedExclusions[acronym]
	if !ok {
		return Exclusion{}, false
	}
	return Exclusion{FieldType: acronym, Reason: reason}, true
}
