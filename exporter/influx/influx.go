package influx

import (
	"context"
	"fmt"
	"time"

	"github.com/hostnetbr/gatekeeper-log-exporter/exporter"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

type Config struct {
	URL             string `yaml:"url"`
	User            string `yaml:"user"`
	Pass            string `yaml:"password"`
	Database        string `yaml:"database"`
	RetentionPolicy string `yaml:"retention_policy"`
	LogLevel        uint   `yaml:"log_level"`
	Hostname        string `yaml:"hostname"`
}

type Exporter struct {
	client influxdb2.Client
	config Config
}

func NewExporter(config Config) Exporter {
	options := influxdb2.DefaultOptions()
	options.SetLogLevel(config.LogLevel)
	client := influxdb2.NewClientWithOptions(config.URL, fmt.Sprintf("%s:%s", config.User, config.Pass), options)
	return Exporter{client, config}
}

func (e Exporter) Export(t time.Time, m *exporter.Measurements) error {
	measurements := measurementsToMap(m)
	p := influxdb2.NewPoint(
		"gkle",
		map[string]string{"host": e.config.Hostname},
		measurements,
		t,
	)

	writeAPI := e.client.WriteAPIBlocking("", fmt.Sprintf("%s/%s", e.config.Database, e.config.RetentionPolicy))
	err := writeAPI.WritePoint(context.Background(), p)
	if err != nil {
		return fmt.Errorf("error writing to influxdb: %w", err)
	}

	return nil
}

func (e Exporter) Close() {
	e.client.Close()
}

func measurementsToMap(ms *exporter.Measurements) map[string]interface{} {
	m := make(map[string]interface{})

	// Parsing as int64 because InfluxDB seems to not support uint64.
	// https://github.com/influxdata/influxdb/issues/9961
	m["tot_pkts_num"] = int64(ms.TotPktsNum)
	m["tot_pkts_size"] = int64(ms.TotPktsSize)
	m["pkts_num_granted"] = int64(ms.PktsNumGranted)
	m["pkts_size_granted"] = int64(ms.PktsSizeGranted)
	m["pkts_num_request"] = int64(ms.PktsNumRequest)
	m["pkts_size_request"] = int64(ms.PktsSizeRequest)
	m["pkts_num_declined"] = int64(ms.PktsNumDeclined)
	m["pkts_size_declined"] = int64(ms.PktsSizeDeclined)
	m["tot_pkts_num_dropped"] = int64(ms.TotPktsNumDropped)
	m["tot_pkts_size_dropped"] = int64(ms.TotPktsSizeDropped)
	m["tot_pkts_num_distributed"] = int64(ms.TotPktsNumDistributed)
	m["tot_pkts_size_distributed"] = int64(ms.TotPktsSizeDistributed)
	m["flow_table_ocupancy_current"] = int64(ms.FlowTableOcupancyCurrent)
	m["flow_table_ocupancy_max"] = int64(ms.FlowTableOcupancyMax)

	return m
}
