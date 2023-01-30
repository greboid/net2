package config

import (
	"encoding/json"
	"errors"
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

func (d *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(*d).String())
}

func (d *Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(*d).String(), nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	out, err := time.ParseDuration(value.Value)
	if err != nil {
		return err
	}
	*d = Duration(out)
	return nil
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
	if config.ClientID == "" {
		return nil, errors.New("clientid is required")
	}
	for index := range config.Sites {
		if config.Sites[index].Port == 0 {
			config.Sites[index].Port = 8080
		}
		if config.Sites[index].ID == -1 {
			return nil, errors.New("id is required for site: " + config.Sites[index].Name)
		}
		if config.Sites[index].Username == "" {
			return nil, errors.New("username is required for site: " + config.Sites[index].Name)
		}
		if config.Sites[index].Password == "" {
			return nil, errors.New("password is required for site: " + config.Sites[index].Name)
		}
		if config.Sites[index].IP == "" {
			return nil, errors.New("ip is required for site: " + config.Sites[index].Name)
		}
		if config.Sites[index].LocalIDField == "" {
			return nil, errors.New("localIDField is required for site: " + config.Sites[index].Name)
		}
	}
	return config, nil
}
