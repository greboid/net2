package config

import "time"

type SiteConfig struct {
	ID                   int             `yaml:"id"`
	Username             string          `yaml:"username"`
	Password             string          `yaml:"password"`
	Name                 string          `yaml:"name"`
	IP                   string          `yaml:"ip"`
	Port                 int             `yaml:"port,omitempty"`
	Https                bool            `yaml:"http,omitempty"`
	LocalIDField         string          `yaml:"localIDField,omitempty"`
	StaffDeptPrefix      string          `yaml:"staffDepartmentPrefix"`
	CleanerDeptPrefix    string          `yaml:"cleaningDepartmentPrefix"`
	ContractorDeptPrefix string          `yaml:"contractorDepartmentsPrefix"`
	VisitorDeptPrefix    string          `yaml:"visitorDepartmentPrefix"`
	CustomerDeptPrefix   string          `yaml:"customerDepartmentPrefix"`
	CancelledDeptPrefix  string          `yaml:"cancelledDepartmentPrefix"`
	MonitoredDoors       []MonitoredDoor `yaml:"monitoredDoors"`
	OpenableDoors        []OpenableDoor  `yaml:"openableDoors"`
}

type MonitoredDoor struct {
	ID   int    `yaml:"id"`
	Name string `yaml:"doorName"`
	Zone string `yaml:"zoneName"`
}

type OpenableDoor struct {
	Name     string         `yaml:"name"`
	Sequence []DoorSequence `yaml:"sequence"`
}

type DoorSequence struct {
	ID       int           `yaml:"id"`
	Duration time.Duration `yaml:"duration"`
}
