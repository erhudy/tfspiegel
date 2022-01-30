package main

import (
	"encoding/json"
	"io/ioutil"
)

func LoadConfig() (Config, error) {
	var config Config
	configData, err := ioutil.ReadFile("config.json")
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(configData, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		panic(err)
	}

	err = MirrorProvidersWithConfig(config)
	if err != nil {
		panic(err)
	}
}
