package main

import (
	"encoding/json"
	"fmt"
	"github.com/rfizzle/collector-helpers/outputs"
	"github.com/rfizzle/syslog-collector/parser"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/tidwall/pretty"
	"gopkg.in/mcuadros/go-syslog.v2"
	"os"
	"os/signal"
	"syscall"
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
		log.Infof("listening on %s/%s", setAddress, "TCP")
		if err := server.ListenTCP(setAddress); err != nil {
			log.Errorf("unable to start TCP listener on %s", setAddress)
			os.Exit(1)
		}
	}

	// Setup UDP listener
	if viper.GetString("protocol") == "udp" || viper.GetString("protocol") == "both" {
		log.Infof("listening on %s/%s", setAddress, "UDP")
		if err := server.ListenUDP(setAddress); err != nil {
			log.Errorf("unable to start UDP listener on %s", setAddress)
			os.Exit(1)
		}
	}

	// Boot up server
	if err := server.Boot(); err != nil {
		log.Errorf("unable to boot syslog service: %v", err.Error())
		os.Exit(1)
	}

	// Setup log writer
	tmpWriter, err := outputs.NewTmpWriter()
	if err != nil {
		log.Errorf("%v", err.Error())
		os.Exit(1)
	}

	// Soft close when CTRL + C is called
	done := setupCloseHandler(tmpWriter, server, channel)

	// Run go routine
	go getEvents(rotationTime, channel, tmpWriter)

	// Infinite wait while server is running
	server.Wait()

	// Wait until closed successfully
	<-done
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
				log.Debugf("temporary log file written to: %v", tmpWriter.LastFilePath)
			}

			// Write to outputs
			if err := outputs.WriteToOutputs(tmpWriter.LastFilePath, timestamp.Format(time.RFC3339)); err != nil {
				log.Errorf("unable to write to output: %v", err)
				log.Errorf("temporary file: %s", tmpWriter.LastFilePath)
				log.Errorf("%v", err)
			}

			// Let know that event has been processes
			log.Infof("%v events processed...", count)

			// Update limit count
			timestamp = time.Now()
			count = 0

			// Remove temp file now
			err := os.Remove(tmpWriter.LastFilePath)
			if err != nil {
				log.Errorf("unable to remove tmp file: %v", err)
			}
		}

		// Define log message
		var logMessage string

		// Check all syslog types
		if logParts["content"] == nil && logParts["message"] == nil {
			continue
		}

		// Get message from syslog struct (map key depends on format)
		if logParts["content"] != nil {
			logMessage = logParts["content"].(string)
		} else {
			logMessage = logParts["message"].(string)
		}

		// Parse content
		if viper.GetString("parser") == "grok" {
			// Construct JSON from GROK patterns
			jsonString, err = parser.ParseGrok(logMessage, viper.GetStringSlice("grok-pattern"))

			// Handle errors in grok parsing
			if err != nil {
				log.Warnf("unable to marshal map to JSON: %v", err)
				continue
			}
		} else if viper.GetString("parser") == "json" {
			// Construct JSON from raw message
			jsonString, err = parser.ParseJson(logMessage)

			// Handle errors in JSON parsing
			if err != nil {
				log.Warnf("unable to parse json message: %v", err)
				continue
			}
		} else if viper.GetString("parser") == "kv" {
			// Construct JSON from KV string
			jsonString, err = parser.ParseKV(logMessage)

			// Handle errors in KV parsing
			if err != nil {
				log.Warnf("unable to parse kv message: %v", err)
				continue
			}
		} else if viper.GetString("parser") == "cef" {
			// Construct JSON from CEF string
			jsonString, err = parser.ParseCef(logMessage)

			// Handle errors in CEF parsing
			if err != nil {
				log.Warnf("unable to parse cef message: %v", err)
				continue
			}
		} else if viper.GetString("parser") == "raw" {
			jsonString, err = json.Marshal(logParts)

			// Handle errors in RAW parsing
			if err != nil {
				log.Warnf("unable to parse raw message: %v", err)
				continue
			}
		}

		if viper.GetString("parser") != "raw" && viper.GetBool("keep-syslog") {
			// Merge message and syslog info
			finalJsonMap := make(map[string]interface{})
			err = json.Unmarshal(jsonString, &finalJsonMap)

			// Handle errors in unmarshal
			if err != nil {
				log.Warnf("unable to unmarshal json results: %v", err)
				continue
			}

			// Loop through syslog info and add to final json object
			for k, v := range logParts {
				if (k == "message" || k == "content") && !viper.GetBool("keep-message") {
					continue
				}
				finalJsonMap[k] = v
			}

			jsonString, err = json.Marshal(finalJsonMap)

			if err != nil {
				log.Error("error marshalling final json: %v", err)
				continue
			}
		}

		// Handle null parse results
		if jsonString == nil {
			log.Error("parse result for syslog message resulted in nil object")
			continue
		}

		// Write to tmp log
		if err :=  tmpWriter.WriteLog(string(pretty.Ugly(jsonString))); err != nil {
			log.Errorf("unable to write log: %v", err)
			continue
		}

		// Increment count to prevent rotating on empty
		count += 1
	}

}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func setupCloseHandler(w *outputs.TmpWriter, s *syslog.Server, channel syslog.LogPartsChannel) chan bool {
	done := make(chan bool)
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Infof("received SIGTERM...")

		// Kill syslog service
		log.Debugf("shutting down syslog service...")
		if err := s.Kill(); err != nil {
			log.Errorf("error closing syslog server: %v", err)
		}

		// Wait until all data has been written
		log.Debugf("waiting for channel to be clear...")
		for len(channel) > 0 {
			<-time.After(time.Duration(1) * time.Second)
		}

		// Close the temp file
		log.Debugf("closing temp file...")
		if err := w.Close(); err != nil {
			log.Errorf("Error closing log file: %v", err)
		}

		// Remove temp file now
		log.Debugf("removing temp file...")
		err := os.Remove(w.LastFilePath)
		if err != nil {
			log.Errorf("Unable to remove tmp file: %v", err)
		}

		// Print success and write to channel
		log.Infof("shutdown successful...")
		done<-true
	}()

	return done
}