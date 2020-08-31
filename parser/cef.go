package parser

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CefEvent struct {
	Version            string
	DeviceVendor       string
	DeviceProduct      string
	DeviceVersion      string
	DeviceEventClassId string
	Name               string
	Severity           string
	Extensions         map[string]string
}

func ParseCef(event string) ([]byte, error) {
	// Split by CEF separator
	arr := strings.Split(event, "|")

	// Split first field to validate CEF
	validate := strings.Split(arr[0], ":")

	// Validate that it is a valid CEF message
	if validate[0] != "CEF" {
		return nil, fmt.Errorf("invalid CEF format")
	}

	// Get extensions
	extensions := strings.Join(arr[7:], "|")

	// Parse extensions in key value format
	keyValueMap, err := parseKeyValue(extensions, true)

	// Build CEF event
	cefEvent := &CefEvent{
		Version:            validate[1],
		DeviceVendor:       cefEscapeField(arr[1]),
		DeviceProduct:      cefEscapeField(arr[2]),
		DeviceVersion:      cefEscapeField(arr[3]),
		DeviceEventClassId: cefEscapeField(arr[4]),
		Name:               cefEscapeField(arr[5]),
		Severity:           cefEscapeField(arr[6]),
		Extensions:         keyValueMap,
	}

	// Marshal JSON string
	jsonString, err := json.Marshal(cefEvent)

	// Handle errors
	if err != nil {
		return nil, err
	}

	return jsonString, nil
}

// Unescape CEF fields
func cefEscapeField(field string) string {

	replacer := strings.NewReplacer(
		"\\\\", "\\",
		"\\|", "|",
		"\\n", "\n",
	)

	return replacer.Replace(field)
}

// Unescape CEF extensions
func cefEscapeExtension(field string) string {

	replacer := strings.NewReplacer(
		"\\\\", "\\",
		"\\n", "\n",
		"\\=", "=",
	)

	return replacer.Replace(field)
}