package main

import (
	"encoding/json"
	"fmt"
	"github.com/rfizzle/collector-helpers/outputs"
	"github.com/spf13/viper"
	"github.com/tidwall/pretty"
	"github.com/vjeantet/grok"
	"gopkg.in/mcuadros/go-syslog.v2"
	"log"
	"time"
)

func main() {
	// Setup Parameters via CLI or ENV
	if err := setupCliFlags(); err != nil {
		log.Fatalf("initialization failed: %v", err.Error())
	}

	// Setup log writer
	tmpWriter, err := outputs.NewTmpWriter()
	if err != nil {
		log.Fatalf("%v\n", err.Error())
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
		server.ListenTCP(setAddress)
	}

	// Setup UDP listener
	if viper.GetString("protocol") == "udp" || viper.GetString("protocol") == "both" {
		server.ListenUDP(setAddress)
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
	count := 0
	timestamp := time.Now()
	g, _ := grok.NewWithConfig(&grok.Config{NamedCapturesOnly: true})

	// Loop through channel
	for logParts := range channel {
		// Rotate file and output if set duration has passed
		if time.Now().After(timestamp.Add(time.Duration(rotationTime) * time.Second)) && count > 0 {

			// Rotate temp file
			_ = tmpWriter.Rotate()

			// Print verbose
			if viper.GetBool("verbose") {
				log.Printf("Log file: %v\n", tmpWriter.LastFilePath)
			}

			// Write to outputs
			if err := outputs.WriteToOutputs(tmpWriter.LastFilePath, timestamp.Format(time.RFC3339)); err != nil {
				log.Fatalf("Unable to write to output: %v", err)
			}

			// Let know that event has been processes
			log.Printf("%v events processed...\n", count)

			// Update limit count
			timestamp = time.Now()
			count = 0
		}

		// Setup grok string
		grokString := viper.GetString("grok-pattern")

		// Parse content
		values, _ := g.Parse(grokString, logParts["content"].(string))

		// Marshal map to json
		jsonString, _ := json.Marshal(values)

		// Write to tmp log
		if err :=  tmpWriter.WriteLog(string(pretty.Ugly(jsonString))); err != nil {
			log.Printf("Error: %v\n", err)
		}

		// Increment count to prevent rotating on empty
		count += 1
	}

}