package exporter

import (
	"time"
)

type Measurements struct {
	TotPktsNum               uint64
	TotPktsSize              uint64
	PktsNumGranted           uint64
	PktsSizeGranted          uint64
	PktsNumRequest           uint64
	PktsSizeRequest          uint64
	PktsNumDeclined          uint64
	PktsSizeDeclined         uint64
	TotPktsNumDropped        uint64
	TotPktsSizeDropped       uint64
	TotPktsNumDistributed    uint64
	TotPktsSizeDistributed   uint64
	FlowTableOcupancyCurrent uint64
	FlowTableOcupancyMax     uint64
}

type Entry struct {
	Time         time.Time
	Lcore        int
	Measurements Measurements
}

type Interface interface {
	Export(t time.Time, m *Measurements) error
	Close()
}
