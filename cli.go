package main

import (
	"errors"
	"github.com/rfizzle/collector-helpers/config"
	"github.com/rfizzle/collector-helpers/outputs"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net"
	"strings"
)

func setupCliFlags() error {
	viper.SetEnvPrefix("SYSLOG_COLLECTOR")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	config.InitCLIParams()
	flag.Int("schedule", 30, "time in seconds to collect")
	flag.String("ip", "", "ip address to listen on")
	flag.Int("port", 1514, "port to listen on")
	flag.String("protocol", "udp", "protocol to use (tcp, udp, both)")
	flag.String("parser", "", "parser to use for syslog messages (grok, json, kv)")
	flag.StringArray("grok-pattern", []string{}, "grok pattern to parse logs to")
	flag.BoolP("verbose", "v", false, "verbose logging")
	outputs.InitCLIParams()
	flag.Parse()
	err := viper.BindPFlags(flag.CommandLine)

	if err != nil {
		return err
	}

	// Check config
	if err := config.CheckConfigParams(); err != nil {
		return err
	}

	// Check parameters
	if err := checkRequiredParams(); err != nil {
		return err
	}

	return nil
}

func checkRequiredParams() error {
	if !validIPAddress(viper.GetString("ip")) {
		return errors.New("invalid ip param (--ip)")
	}

	if viper.GetInt("port") < 0 || viper.GetInt("port") > 65535 {
		return errors.New("invalid port param (--port)")
	}

	if !contains([]string{"tcp", "udp", "both"}, viper.GetString("protocol")) {
		return errors.New("invalid protocol param (--protocol)")
	}

	if !contains([]string{"grok", "json", "kv"}, viper.GetString("parser")) {
		return errors.New("invalid parser param (--parser)")
	}

	if viper.GetString("parser") == "grok" && len(viper.GetStringSlice("grok-pattern")) == 0 {
		return errors.New("invalid grok-pattern param (--grok-pattern)")
	}

	if err := outputs.ValidateCLIParams(); err != nil {
		return err
	}

	return nil
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func validIPAddress(ip string) bool {
	if net.ParseIP(ip) == nil {
		return false
	} else {
		return true
	}
}