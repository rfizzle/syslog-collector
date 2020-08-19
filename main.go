package main

import (
	"encoding/json"
	"fmt"
	"github.com/rfizzle/collector-helpers/outputs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/tidwall/pretty"
	"github.com/vjeantet/grok"
	"gopkg.in/mcuadros/go-syslog.v2"
	"os"
	"time"
)

func main() {
	// Setup logging
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	log.SetOutput(os.Stdout)

	// Setup Parameters via CLI or ENV
	if err := setupCliFlags(); err != nil {
		log.Errorf("initialization failed: %v", err.Error())
		os.Exit(1)
	}

	// Set log level based on supplied verbosity
	if viper.GetBool("verbose") {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// Setup log writer
	tmpWriter, err := outputs.NewTmpWriter()
	if err != nil {
		log.Errorf("%v\n", err.Error())
		os.Exit(1)
	}

	// Setup the rotation time
	rotationTime := viper.GetInt("schedule")

	// Setup Channel
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	// Setup syslog server
	server := syslog.NewServer()
	server.SetFormat(syslog.Automatic)
	server.SetHandler(handler)

	// Get address from supplied parameters
	setAddress := fmt.Sprintf("%s:%d", viper.GetString("ip"), viper.GetInt("port"))

	// Setup TCP listener
	if viper.GetString("protocol") == "tcp" || viper.GetString("protocol") == "both" {
		log.Infof("syslog-collector listening on %s/%s", setAddress, "TCP")
		if err = server.ListenTCP(setAddress); err != nil {
			log.Errorf("unable to start TCP listener on %s\n", setAddress)
			os.Exit(1)
		}
	}

	// Setup UDP listener
	if viper.GetString("protocol") == "udp" || viper.GetString("protocol") == "both" {
		log.Infof("syslog-collector listening on %s/%s", setAddress, "UDP")
		if err = server.ListenUDP(setAddress); err != nil {
			log.Errorf("unable to start UDP listener on %s\n", setAddress)
			os.Exit(1)
		}
	}

	// Boot up server
	server.Boot()

	// Run go routine
	go getEvents(rotationTime, channel, tmpWriter)

	// Infinite wait
	server.Wait()
}

// Get events
func getEvents(rotationTime int, channel syslog.LogPartsChannel, tmpWriter *outputs.TmpWriter) {
	// Setup required variables
	var err error
	var jsonString []byte
	count := 0
	timestamp := time.Now()

	// Loop through channel
	for logParts := range channel {
		// Rotate file and output if set duration has passed
		if time.Now().After(timestamp.Add(time.Duration(rotationTime) * time.Second)) && count > 0 {
			// Rotate temp file
			_ = tmpWriter.Rotate()

			// Print verbose
			if viper.GetBool("verbose") {
				log.Debugf("Temporary log file written to: %v\n", tmpWriter.LastFilePath)
			}

			// Write to outputs
			if err := outputs.WriteToOutputs(tmpWriter.LastFilePath, timestamp.Format(time.RFC3339)); err != nil {
				log.Errorf("Unable to write to output: %v\n", err)
				log.Errorf("Temporary file: %s\n", tmpWriter.LastFilePath)
				log.Errorf("%v\n", err)
			}

			// Let know that event has been processes
			log.Infof("%v events processed...\n", count)

			// Update limit count
			timestamp = time.Now()
			count = 0

			// Remove temp file now
			err := os.Remove(tmpWriter.LastFilePath)
			if err != nil {
				log.Errorf("Unable to remove tmp file: %v", err)
			}
		}

		// Parse content
		if viper.GetString("parser") == "grok" {
			g, _ := grok.NewWithConfig(&grok.Config{NamedCapturesOnly: true})
			var values map[string]string

			// Setup grok string
			for _, v := range viper.GetStringSlice("grok-pattern") {
				values, err = g.Parse(v, logParts["content"].(string))
				if err == nil {
					break
				}
			}

			// If error parsing, print error and skip to next
			if err != nil {
				log.Warnf("Unable to parse: %v", err)
				continue
			}

			// Marshal map to json
			jsonString, err = json.Marshal(values)

			// Print error
			if err != nil {
				log.Errorf("Unable to marshal map to JSON: %v", err)
				continue
			}
		} else if viper.GetString("parser") == "json" {
			jsonString = []byte(logParts["content"].(string))
		}

		// Handle null parse results
		if jsonString == nil {
			log.Error("Parse result for syslog message resulted in null object")
			continue
		}

		// Write to tmp log
		if err :=  tmpWriter.WriteLog(string(pretty.Ugly(jsonString))); err != nil {
			log.Errorf("Unable to write log: %v\n", err)
			continue
		}

		// Increment count to prevent rotating on empty
		count += 1
	}

}