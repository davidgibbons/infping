// infping.go copyright Tor Hveem
// License: MIT

package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/influxdata/influxdb/client/v2"
	"github.com/spf13/viper"
)

func main() {
	viper.SetDefault("influx.host", "localhost")
	viper.SetDefault("influx.port", "8086")
	viper.SetDefault("influx.user", "")
	viper.SetDefault("influx.pass", "")
	viper.SetDefault("influx.secure", false)
	viper.SetDefault("influx.db", "infping")
	viper.SetDefault("fping.backoff", "1")
	viper.SetDefault("fping.retries", "0")
	viper.SetDefault("fping.tos", "0")
	viper.SetDefault("fping.summary", "10")
	viper.SetDefault("fping.period", "1000")
	viper.SetDefault("hosts.hosts", []string{"localhost"})
	viper.SetDefault("fping.custom", map[string]string{})
	viper.SetDefault("hosts.hosts", []string{"localhost"})

	viper.SetConfigName("infping")
	viper.AddConfigPath("/etc/")
	viper.AddConfigPath("/usr/local/etc/")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Unable to read config file", err)
	}

	influxScheme := "https"
	if !viper.GetBool("influx.secure") {
		influxScheme = "http"
	}
	influxHost := viper.GetString("influx.host")
	influxPort := viper.GetString("influx.port")
	influxUser := viper.GetString("influx.user")
	influxPass := viper.GetString("influx.pass")
	influxDB := viper.GetString("influx.db")
	influxRetPolicy := viper.GetString("influx.policy")

	hosts := viper.GetStringSlice("hosts.hosts")

	u, err := url.Parse(fmt.Sprintf("%s://%s:%s", influsScheme, influxHost, influxPort))
	if err != nil {
		log.Fatal("Unable to build valid Influx URL", err)
	}

	conf := client.HTTPConfig{
		Addr:     u.String(),
		Username: influxUser,
		Password: influxPass,
	}

	rawClient, err := client.NewHTTPClient(conf)
	if err != nil {
		log.Fatal("Failed to create Influx client", err)
	}

	influxClient := NewInfluxClient(rawClient, influxDB, influxRetPolicy)

	dur, version, err := influxClient.Ping()
	if err != nil {
		log.Fatal("Unable to ping InfluxDB", err)
	}
	log.Printf("Pinged InfluxDB (version %s) in %v", version, dur)

	q := client.Query{
		Command: "SHOW DATABASES",
	}
	databases, err := influxClient.Query(q)
	if err != nil {
		log.Fatal("Unable to list databases", err)
	}
	if len(databases.Results) != 1 {
		log.Fatalf("Expected 1 result in response, got %d", len(databases.Results))
	}
	if len(databases.Results[0].Series) != 1 {
		log.Fatalf("Expected 1 series in result, got %d", len(databases.Results[0].Series))
	}
	found := false
	for i := 0; i < len(databases.Results[0].Series[0].Values); i++ {
		if databases.Results[0].Series[0].Values[i][0] == influxDB {
			found = true
		}
	}
	if !found {
		q = client.Query{
			Command: fmt.Sprintf("CREATE DATABASE %s", influxDB),
		}
		_, err := influxClient.Query(q)
		if err != nil {
			log.Fatalf("Failed to create database %s %v", influxDB, err)
		}
		log.Printf("Created new database %s", influxDB)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fpingBackoff := viper.GetString("fping.backoff")
	fpingRetries := viper.GetString("fping.retries")
	fpingTos := viper.GetString("fping.tos")
	fpingSummary := viper.GetString("fping.summary")
	fpingPeriod := viper.GetString("fping.period")
	fpingConfig := map[string]string{
		"-B": fpingBackoff,
		"-r": fpingRetries,
		"-O": fpingTos,
		"-Q": fpingSummary,
		"-p": fpingPeriod,
		"-l": "",
		"-D": "",
	}
	fpingCustom := viper.GetStringMapString("fping.custom")
	for k, v := range fpingCustom {
		fpingConfig[k] = v
	}

	log.Printf("Launching fping with hosts: %s", strings.Join(hosts, ", "))
	runAndRead(ctx, hosts, influxClient, fpingConfig)
}
