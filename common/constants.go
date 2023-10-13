package common

type ZoneType string

const (
	ZoneTypeDCGroup           = ZoneType("dcgroup")
	ZoneTypeUserSpecifiedEdge = ZoneType("userspecifiededge")
	ZoneTypeExternal          = ZoneType("external")
)
