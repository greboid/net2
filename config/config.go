package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	APIPort  int          `yaml:"apiport"`
	ClientID string       `yaml:"clientid"`
	Sites    []SiteConfig `yaml:"sites"`
}

func LoadConfig(file string) (*Config, error) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = yaml.Unmarshal(bytes, config)
	if err != nil {
		return nil, err
	}
	if config.APIPort == 0 {
		config.APIPort = 8000
	}
	for index := range config.Sites {
		if config.Sites[index].Port == 0 {
			config.Sites[index].Port = 8080
		}
	}
	return config, nil
}
