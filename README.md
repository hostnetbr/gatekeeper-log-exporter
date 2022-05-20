# Gatekeeper Log Exporter

Gatekeeper Log Exporter (GKLE for shorten) provides an easy way to aggregate and export logs from <a href="https://github.com/AltraMayor/gatekeeper" target="_blank">Gatekeeper</a>.

## How it Works

GKLE works by listening to the Gatekeeper log directory and processing a complete log file every time a new one is generated.

While processing the log file, GKLE agreggates the lcore separated data and exports it (currently it only supports InfluxDB).

## How to Set Up

### Config file

A config file should be located at `/etc/gkle.yaml`. GKLE uses it to read the Gatekeeper log directory and get InfluxDB credentials.

The config file uses the following format:

```
gk_log_dir: ""

influxdb:
  url:              ""
  user:             ""
  password:         ""
  database:         ""
  retention_policy: ""
  log_level :       0
  hostname:         ""
```

`gk_log_dir` option receives the directory where gatekeeper is logging data (usually `/var/log/gatekeeper/`).

`influxdb` option receives: connection URL, username and password, the desired database and retention policy, the log level (0 to 3, as described <a href="https://pkg.go.dev/github.com/influxdata/influxdb-client-go#Options.SetLogLevel" target="_blank">here</a>), and finally the hostname of the server where Gatekeeper is running.

### Running 

GKLE should be compiled and executed from systemd or another init system, so it can run on background listening to the files being created on the log directory.
