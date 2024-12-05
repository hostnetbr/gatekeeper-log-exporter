package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/hostnetbr/gatekeeper-log-exporter/exporter"
	"github.com/hostnetbr/gatekeeper-log-exporter/exporter/influx"

	"github.com/fsnotify/fsnotify"
)

const (
	confFile    = "/etc/gkle.yaml"
	lastLogFile = "/var/lib/gkle/last"
	timeLayout  = "2006-01-02 15:04:05"
)

var logLineRegex = regexp.MustCompile(`^GK\/(\d+)\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+NOTICE\s+Basic\s+measurements\s+\[tot_pkts_num\s+=\s+(\d+),\s+tot_pkts_size\s+=\s+(\d+),\s+pkts_num_granted\s+=\s+(\d+),\s+pkts_size_granted\s+=\s+(\d+),\s+pkts_num_request\s+=\s+(\d+),\s+pkts_size_request\s+=\s+(\d+),\s+pkts_num_declined\s+=\s+(\d+),\s+pkts_size_declined\s+=\s+(\d+),\s+tot_pkts_num_dropped\s+=\s+(\d+),\s+tot_pkts_size_dropped\s+=\s+(\d+),\s+tot_pkts_num_distributed\s+=\s+(\d+),\s+tot_pkts_size_distributed\s+=\s+(\d+),\s+flow_table_occupancy\s+=\s+(\d+)\/(\d+)=\d+\.\d+%]`)
var logFileRegex = regexp.MustCompile(`gatekeeper_\d{4}_\d{2}_\d{2}_\d{2}_\d{2}.log`)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := parseConfig()
	if err != nil {
		slog.Error(fmt.Sprintf("error reading config file: %v\n", err))
		return 1
	}

	if cfg.LogLineRegex != "" {
		logLineRegex = regexp.MustCompile(cfg.LogLineRegex)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error(fmt.Sprintf("error creating fsnotify watcher: %v\n", err))
		return 1
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					filesToParse, err := getFilesToParse(cfg.GkLogDir)
					if err != nil {
						slog.Error(fmt.Sprintf("error getting log files to parse: %v", err))
						return
					}

					for _, file := range filesToParse {
						ex := influx.NewExporter(*cfg.InfluxDB)
						parseLogFile(file, ex)
						ex.Close()

						if err := saveLastLog(file); err != nil {
							slog.Error(fmt.Sprintf("error saving last read log: %v", err))
							return
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error(fmt.Sprintf("error while watching log dir: %v\n", err))
			}
		}
	}()

	err = watcher.Add(cfg.GkLogDir)
	if err != nil {
		slog.Error(fmt.Sprintf("error adding watcher to log dir: %s. %v\n", cfg.GkLogDir, err))
		return 1
	}
	<-done

	return 0
}

func getFilesToParse(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("error reading gatekeeper log dir %s: %w", path, err)
	}

	parseAll := false
	lastParsedLog, err := os.ReadFile(lastLogFile)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("parsing all log files")
			parseAll = true
		} else {
			return nil, fmt.Errorf("error reading last log file: %w", err)
		}
	}

	var files []string
	var lastParsedLogPosition int
	// We don't parse the last file of the directory because it's still being
	// written by gatekeeper.
	for _, entry := range entries {
		if !entry.IsDir() && logFileRegex.MatchString(entry.Name()) {
			fileName := filepath.Join(path, entry.Name())
			files = append(files, fileName)
			if fileName == string(lastParsedLog) {
				lastParsedLogPosition = len(files) - 1
			}
		}
	}

	if parseAll || lastParsedLogPosition == 0 {
		return files[:len(files)-1], nil
	}

	return files[lastParsedLogPosition+1 : len(files)-1], nil
}

func parseLogFile(filename string, ex exporter.Interface) error {
	slog.Debug(fmt.Sprintf("parsing log file %s", filename))

	logFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}

	defer logFile.Close()

	fileScanner := bufio.NewScanner(logFile)
	fileScanner.Split(bufio.ScanLines)

	err = match(fileScanner, ex.Export)
	if err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	return nil
}

var errNoMatch = errors.New("line does not match")

func match(sc *bufio.Scanner, f func(time.Time, *exporter.Measurements) error) error {
	entries := make(map[int]exporter.Entry)
	for sc.Scan() {
		line := sc.Text()
		entry, err := parseEntry(line)
		if err != nil {
			// line doesn't match up with regex; ignornig
			if err == errNoMatch {
				continue
			}
			return fmt.Errorf("error parsing entry: %w", err)
		}
		if _, repeat := entries[entry.Lcore]; repeat {
			// repeated lcore; proccessing previous minute and starting new one
			time, aggr := aggregate(entries)
			if err := f(time, &aggr); err != nil {
				return fmt.Errorf("error exporting data: %w", err)
			}
			entries = make(map[int]exporter.Entry)
		}

		entries[entry.Lcore] = entry
	}
	if sc.Err() == nil {
		// EOF
		time, aggr := aggregate(entries)
		if err := f(time, &aggr); err != nil {
			return fmt.Errorf("error exporting data: %w", err)
		}
	}
	return nil
}

func parseEntry(line string) (exporter.Entry, error) {
	matches := logLineRegex.FindStringSubmatch(line)
	if matches == nil {
		return exporter.Entry{}, errNoMatch
	}

	logTime, err := time.Parse(timeLayout, matches[2])
	if err != nil {
		return exporter.Entry{}, fmt.Errorf("error parsing log time: %w", err)
	}

	lcore, err := strconv.Atoi(matches[1])
	if err != nil {
		return exporter.Entry{}, fmt.Errorf("error parsing lcore: %w", err)
	}

	measurements := exporter.Measurements{
		TotPktsNum:               mustParseUint(matches[3]),
		TotPktsSize:              mustParseUint(matches[4]),
		PktsNumGranted:           mustParseUint(matches[5]),
		PktsSizeGranted:          mustParseUint(matches[6]),
		PktsNumRequest:           mustParseUint(matches[7]),
		PktsSizeRequest:          mustParseUint(matches[8]),
		PktsNumDeclined:          mustParseUint(matches[9]),
		PktsSizeDeclined:         mustParseUint(matches[10]),
		TotPktsNumDropped:        mustParseUint(matches[11]),
		TotPktsSizeDropped:       mustParseUint(matches[12]),
		TotPktsNumDistributed:    mustParseUint(matches[13]),
		TotPktsSizeDistributed:   mustParseUint(matches[14]),
		FlowTableOcupancyCurrent: mustParseUint(matches[15]),
		FlowTableOcupancyMax:     mustParseUint(matches[16]),
	}

	entry := exporter.Entry{
		Time:         logTime,
		Lcore:        lcore,
		Measurements: measurements,
	}
	return entry, nil
}

func mustParseUint(s string) uint64 {
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return u
}

func aggregate(entries map[int]exporter.Entry) (time.Time, exporter.Measurements) {
	var time time.Time
	var aggr exporter.Measurements

	for _, e := range entries {
		time = e.Time
		m := e.Measurements

		aggr.TotPktsNum += m.TotPktsNum
		aggr.TotPktsSize += m.TotPktsSize
		aggr.PktsNumGranted += m.PktsNumGranted
		aggr.PktsSizeGranted += m.PktsSizeGranted
		aggr.PktsNumRequest += m.PktsNumRequest
		aggr.PktsSizeRequest += m.PktsSizeRequest
		aggr.PktsNumDeclined += m.PktsNumDeclined
		aggr.PktsSizeDeclined += m.PktsSizeDeclined
		aggr.TotPktsNumDropped += m.TotPktsNumDropped
		aggr.TotPktsSizeDropped += m.TotPktsSizeDropped
		aggr.TotPktsNumDistributed += m.TotPktsNumDistributed
		aggr.TotPktsSizeDistributed += m.TotPktsSizeDistributed
		aggr.FlowTableOcupancyCurrent += m.FlowTableOcupancyCurrent
		aggr.FlowTableOcupancyMax += m.FlowTableOcupancyMax
	}
	return time, aggr
}

func saveLastLog(logFile string) error {
	dir, err := os.OpenFile(filepath.Dir(lastLogFile), os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("open directory failed: %w", err)
	}
	defer dir.Close()

	tmp := fmt.Sprintf("%s.tmp", lastLogFile)
	if err := safeWrite(tmp, logFile); err != nil {
		return fmt.Errorf("safe write failed: %w", err)
	}
	if err := os.Rename(tmp, lastLogFile); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync directory failed: %w", err)
	}

	return nil
}

func safeWrite(path string, data string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create failed: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(data); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync file failed: %w", err)
	}

	return nil
}
