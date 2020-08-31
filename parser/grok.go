package parser

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"github.com/vjeantet/grok"
)

func ParseGrok(event string, grokPatterns []string) ([]byte, error) {
	// Setup grok
	g, err := grok.NewWithConfig(&grok.Config{NamedCapturesOnly: true})

	// Handle errors
	if err != nil {
		log.Errorf("unable to setup grok parser: %v", err)
	}

	// Setup values map
	var values map[string]string

	// Setup grok string (this will loop through all the patterns until one works or all fail)
	for _, v := range grokPatterns {
		values, err = g.Parse(v, event)
		if err == nil {
			break
		}
	}

	// If none of the patterns worked, print error and skip to next
	if err != nil {
		log.Warnf("unable to parse: %v", err)
		return nil, err
	}

	// Marshal map to json
	return json.Marshal(values)
}
