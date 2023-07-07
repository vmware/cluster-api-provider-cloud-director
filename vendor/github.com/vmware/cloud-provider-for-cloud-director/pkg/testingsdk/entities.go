package testingsdk

type Status struct {
	CAPVCDStatus map[string]interface{} `json:"capvcd,omitempty"`
}

type CAPVCDEntity struct {
	Status Status `json:"status"`
}

type FullCAPVCDEntity struct {
	Entity CAPVCDEntity `json:"entity"`
}
