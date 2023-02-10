package net2

import (
	"github.com/go-co-op/gocron"
	"github.com/greboid/net2/config"
	"github.com/rs/zerolog"
	"net/http"
	"sync"
	"time"
)

const (
	DoorStatus_NoFlag            = 0
	DoorStatus_IntruderAlarm     = 1
	DoorStatus_PSUIsOK           = 2
	DoorStatus_TamperStatusGood  = 3
	DoorStatus_DoorContactClosed = 4
	DoorStatus_AlarmTripped      = 10
	DoorStatus_DoorOpen          = 20
)

type Site struct {
	portalIDField    int
	logger           *zerolog.Logger
	httpClient       *http.Client
	cron             *gocron.Scheduler
	config           *config.SiteConfig
	localIDFieldName string
	updateLock       sync.Mutex
	clientID         string
	LocalIDField     string                         `json:"-"`
	AccessLevels     map[int]*AccessLevel           `json:"-"`
	Departments      map[int]*Department            `json:"-"`
	Users            map[int]*User                  `json:"-"`
	Fields           map[int]*CustomFieldDefinition `json:"-"`
	QuitChan         chan bool                      `json:"-"`
	BaseURL          string                         `json:"-"`
	Doors            map[uint64]*Door               `json:"-"`
	UnknownTokens    []Event                        `json:"-"`
	SiteID           int                            `json:"ID"`
	Name             string                         `json:"Name"`
	LastPolled       time.Time                      `json:"lastPolled"`
}

type AccessLevel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Area struct {
	ID   int    `json:"areaID"`
	Name string `json:"name"`
}

type Permission struct {
	AccessLevels          []int         `json:"accessLevels"`
	IndividualPermissions []AccessLevel `json:"individualPermissions"`
}

type Department struct {
	ID   int    `json:"Id"`
	Name string `json:"Name"`
}

type User struct {
	GUID              string            `json:"UserGUID"`
	ID                int               `json:"id"`
	Activated         time.Time         `json:"activateDate"`
	Expiry            time.Time         `json:"expiryDate"`
	FirstName         string            `json:"firstName"`
	Surname           string            `json:"lastName"`
	Custom            []UserCustomField `json:"customFields,omitempty"`
	PIN               string            `json:"pin"`
	Departments       []Department      `json:"department"`
	LocalID           string
	LastKnownLocation string
	LastUpdated       time.Time
	AccessLevels      []string
}

type UserCustomField struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

type CustomFieldDefinition struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      int    `json:"type"`
	MaxLength int    `json:"maxLength"`
}

type Door struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	StatusFlag  int
	AlarmStatus int
	AlarmZone   string
}

type DoorSequenceItem struct {
	Door uint64        `json:"door"`
	Time time.Duration `json:"time"`
}

type Event struct {
	Date     time.Time `json:"EventDate"`
	Location string    `json:"where"`
	Token    int64     `json:"tokenNumber"`
}

type userSQLQuery struct {
	UserGUID        string `json:"UserGUID"`
	ID              int    `json:"userID"`
	Firstname       string `json:"FirstName"`
	Surname         string `json:"Surname"`
	ActivateDate    string `json:"ActivateDate"`
	ExpiryDate      string `json:"ExpiryDate"`
	PIN             string `json:"PIN"`
	LastLocation    string `json:"lastKnownLocation"`
	LastAccessTime  string `json:"lastAccessTime"`
	DepartmentID    int    `json:"DepartmentID"`
	DepartmentName  string `json:"DepartmentName"`
	AccessLevelName string `json:"AccessLevelName"`
	LocalID         string `json:"LocalID"`
}

type deviceSQLQuery struct {
	ID     int `json:"Address"`
	Status int `json:"StatusFlag"`
}

type userToken struct {
	ID         int    `json:"id"`
	TokenType  string `json:"tokenType"`
	TokenValue string `json:"tokenValue"`
	Lost       bool   `json:"isLost"`
}
