package net2

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-co-op/gocron"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	JsonContentType = "application/json"
)

//go:embed nophoto.png
var nophoto []byte

func (s *Site) Start() error {
	s.Users = make(map[int]*User, 0)
	s.Departments = make(map[int]*Department)
	s.AccessLevels = make(map[int]*AccessLevel)
	s.Doors = make(map[uint64]*Door)
	s.LocalIDField = s.getLocalFieldName()
	if s.cron == nil {
		s.cron = gocron.NewScheduler(time.Now().Location())
	}
	_, err := s.cron.Every("1m").Tag("users").Do(func() {
		s.UpdateThing("users", s.UpdateUsers)
	})
	_, err = s.cron.Every("1m").Tag("accesslevels").Do(func() {
		s.UpdateThing("accesslevels", s.UpdateAccessLevels)
	})
	_, err = s.cron.Every("1m").Tag("doors").Do(func() {
		s.UpdateThing("doors", s.UpdateDoors)
	})
	_, err = s.cron.Every("1m").Tag("departments").Do(func() {
		s.UpdateThing("departments", s.UpdateDepartments)
	})
	s.cron.StartAsync()
	return err
}

func (s *Site) Stop() {
	s.cron.Stop()
}

func (s *Site) GetUser(userID int) *User {
	return s.Users[userID]
}

func (s *Site) GetUsers() map[int]*User {
	return s.Users
}

func (s *Site) GetUserPicture(userID int) ([]byte, error) {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s/api/v1/users/%d/image", s.BaseURL, userID))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nophoto, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("user not found")
	}
	body, err := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	return body, nil
}

func (s *Site) GetActiveUsersInDepartment(prefix string) map[int]*User {
	return lo.PickBy(s.Users, func(_ int, user *User) bool {
		return lo.CountBy(user.Departments, func(department Department) bool {
			return strings.HasPrefix(department.Name, prefix)
		}) > 0
	})
}

func (s *Site) GetTodaysActiveUsersInDepartment(prefix string) map[int]*User {
	today := time.Now()
	midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	return lo.PickBy(s.Users, func(_ int, user *User) bool {
		return lo.CountBy(user.Departments, func(department Department) bool {
			return strings.HasPrefix(department.Name, prefix)
		}) > 0 && user.LastUpdated.After(midnight)
	})
}

func (s *Site) GetActiveStaffToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(s.config.StaffDeptPrefix)
}

func (s *Site) GetActiveStaff() map[int]*User {
	return s.GetActiveUsersInDepartment(s.config.StaffDeptPrefix)
}

func (s *Site) GetActiveVisitorsToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(s.config.VisitorDeptPrefix)
}

func (s *Site) GetActiveVisitors() map[int]*User {
	return s.GetActiveUsersInDepartment(s.config.VisitorDeptPrefix)
}

func (s *Site) GetActiveUsersToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment("")
}

func (s *Site) GetActiveUsers() map[int]*User {
	return s.GetActiveUsersInDepartment("")
}

func (s *Site) GetDoors() map[uint64]*Door {
	return s.Doors
}

func (s *Site) GetDoor(doorID uint64) *Door {
	return s.Doors[doorID]
}

func (s *Site) OpenDoor(doorID uint64) error {
	jsonBytes, _ := json.Marshal(map[string]uint64{"doorId": doorID})
	_, ok := s.Doors[doorID]
	if !ok {
		return errors.New("invalid door")
	}
	resp, err := s.httpClient.Post(fmt.Sprintf("%s/api/v1/commands/door/open", s.BaseURL), JsonContentType, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New("unable to open door")
	}
	return nil
}

func (s *Site) CloseDoor(doorID uint64) error {
	jsonBytes, _ := json.Marshal(map[string]uint64{"doorId": doorID})
	_, ok := s.Doors[doorID]
	if !ok {
		return errors.New("invalid door")
	}
	resp, err := s.httpClient.Post(fmt.Sprintf("%s/api/v1/commands/door/close", s.BaseURL), JsonContentType, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New("unable to close door")
	}
	return nil
}

func (s *Site) GetAccessLevels() map[int]*AccessLevel {
	return s.AccessLevels
}

func (s *Site) GetDepartments() map[int]*Department {
	return s.Departments
}

func (s *Site) ResetAntiPassback(userID int) error {
	jsonBytes, _ := json.Marshal(map[string]int{"userId": userID})
	resp, err := s.httpClient.Post(fmt.Sprintf("%s/api/v1/commands/antipassback/reset", s.BaseURL), JsonContentType, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New("unable to reset anti passback")
	}
	return nil
}

func (s *Site) ActivateUser(userID int) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.Local)
	return s.UpdateUserInfo(userID, map[string]interface{}{
		"ExpiryDate": today,
	})
}

func (s *Site) DeactivateUser(userID int) error {
	now := time.Now()
	yesterday := time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 59, 0, time.Local)
	return s.UpdateUserInfo(userID, map[string]interface{}{
		"ExpiryDate": yesterday,
	})
}

func (s *Site) UpdateUserInfo(userID int, info map[string]interface{}) error {
	info["Id"] = userID
	if _, ok := info["ExpiryDate"]; !ok {
		info["ExpiryDate"] = s.Users[userID].Expiry
	}
	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Post(fmt.Sprintf("%s/api/v1/users/%d", s.BaseURL, userID), JsonContentType, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != 200 {
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to update user")
		return errors.New("unable to update user info")
	}
	return s.UpdateUser(userID)
}

func (s *Site) ChangeUserDepartment(userID, departmentID int) error {
	newDepartment, ok := s.Departments[departmentID]
	if !ok {
		return fmt.Errorf("department not found: %d", departmentID)
	}
	jsonBytes, err := json.Marshal(newDepartment)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Post(fmt.Sprintf("%s/api/v1/users/%d/departments", s.BaseURL, userID), JsonContentType, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 {
		s.logger.Error().Err(err).Msg("Unable to update user info")
		return errors.New("unable to update user info")
	}
	return s.UpdateUser(userID)
}

func (s *Site) UpdateUserAccessLevels(userID int, accesslevels []int) error {
	info := Permission{
		AccessLevels:          accesslevels,
		IndividualPermissions: []AccessLevel{},
	}
	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Post(fmt.Sprintf("%s/api/v1/users/%d/doorpermissionset", s.BaseURL, userID), JsonContentType, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyData, _ := io.ReadAll(resp.Body)
		defer func() {
			_ = resp.Body.Close()
		}()
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to update user access level")
		return errors.New("unable to update user info")
	}
	return s.UpdateUser(userID)
}

func (s *Site) UpdateUserAccessLevel(userID int, accesslevel int) error {
	var newAccessLevel []int
	if accesslevel == -1 {
		newAccessLevel = []int{0}
	} else {
		newAccessLevel = []int{accesslevel}
	}
	return s.UpdateUserAccessLevels(userID, newAccessLevel)
}

func (s *Site) SetUserAccessLevel(userID int, accesslevel int) error {
	newLevels := []int{accesslevel}
	return s.UpdateUserAccessLevels(userID, newLevels)
}

func (s *Site) getAccessLevelIDByName(levelName string) int {
	for id := range s.AccessLevels {
		if s.AccessLevels[id].Name == levelName {
			return s.AccessLevels[id].ID
		}
	}
	return -1
}

func (s *Site) RemoveUserAccessLevel(userID int, accesslevel int) error {
	existingLevelNames := s.Users[userID].AccessLevels
	newLevels := make([]int, 0)
	for index := range existingLevelNames {
		key := s.getAccessLevelIDByName(existingLevelNames[index])
		if key != accesslevel {
			newLevels = append(newLevels, key)
		}
	}
	return s.UpdateUserAccessLevels(userID, newLevels)
}

func (s *Site) AddUserAccessLevel(userID int, accesslevel int) error {
	existingLevelNames := s.Users[userID].AccessLevels
	newLevels := make([]int, 0)
	for index := range existingLevelNames {
		newLevels = append(newLevels, s.getAccessLevelIDByName(existingLevelNames[index]))
	}
	newLevels = append(newLevels, accesslevel)
	return s.UpdateUserAccessLevels(userID, newLevels)
}

func (s *Site) SequenceDoor(items ...DoorSequenceItem) {
	for _, value := range items {
		_ = s.OpenDoor(value.Door)
		time.Sleep(value.Time)
	}
}

func (s *Site) UpdateUserNameAndExpiryAndAccessLevel(userid int, firstname string, surname string, expiry time.Time, level int) error {
	err := s.UpdateUserInfo(userid, map[string]interface{}{
		"FirstName":  firstname,
		"LastName":   surname,
		"ExpiryDate": expiry,
	})
	if err != nil {
		return err
	}
	err = s.UpdateUserAccessLevel(userid, level)
	if err != nil {
		return err
	}
	return nil
}

func (s *Site) UpdateAll() {
	s.UpdateThing("users", s.UpdateUsers)
	s.UpdateThing("accesslevels", s.UpdateAccessLevels)
	s.UpdateThing("doors", s.UpdateDoors)
	s.UpdateThing("departments", s.UpdateDepartments)
}

func (s *Site) UpdateThing(description string, fn func() error) {
	log.Debug().Str("Site", s.Name).Msg("Updating " + description)
	err := fn()
	if err != nil {
		log.Error().Err(err).Str("Site", s.Name).Msg("Error updating " + description)
	} else {
		log.Debug().Str("Site", s.Name).Msg("Updated " + description)
	}
}

func (s *Site) getLocalFieldName() string {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/users/customfieldnames"))
	if err != nil {
		return ""
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		log.Error().Bytes("Response", bodyData).Msg("Unable to get custom fields")
		return ""
	}
	fields := make([]*CustomFieldDefinition, 20)
	err = json.Unmarshal(bodyData, &fields)
	if err != nil {
		log.Error().Err(err).Msg("Unable to unmarshall custom fields")
		return ""
	}
	if s.localIDFieldName == "" {
		return ""
	}
	for index := range fields {
		if fields[index].Name == s.localIDFieldName {
			if fields[index].ID == 1 || fields[index].ID == 2 {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "100")
			} else if fields[index].ID == 6 || fields[index].ID == 7 {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "60")
			} else if fields[index].ID == 13 {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "Memo")
			} else {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "50")
			}
		}
	}
	return ""
}

func (s *Site) UpdateUser(userID int) error {
	return s.updateUsersWithData(fmt.Sprintf("SELECT *, %s as LocalID FROM UsersEx WHERE userID=%d", s.LocalIDField, userID))
}

func (s *Site) UpdateUsers() error {
	return s.updateUsersWithData(fmt.Sprintf("SELECT *, %s as LocalID FROM UsersEx", s.LocalIDField))
}

func (s *Site) updateUsersWithData(query string) error {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s/api/v1/customquery/querydb?query=%s", s.BaseURL, url.QueryEscape(query)))
	if err != nil {
		return err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to pull users")
		return errors.New("unable to update user info")
	}
	data := make([]*userSQLQuery, 0)
	err = json.Unmarshal(bodyData, &data)
	if err != nil {
		return err
	}
	for id := range data {
		if _, ok := s.Users[id]; !ok {
			s.Users[id] = &User{}
		}
		s.Users[id].ID = data[id].ID
		s.Users[id].Activated = data[id].ActivateDate
		s.Users[id].Expiry = data[id].ExpiryDate
		s.Users[id].FirstName = data[id].Firstname
		s.Users[id].Surname = data[id].Surname
		s.Users[id].PIN = data[id].PIN
		s.Users[id].GUID = data[id].UserGUID
		if updatedTime, err := time.ParseInLocation("2006-01-02T15:04:05", data[id].LastAccessTime, time.Local); err == nil {
			s.Users[id].LastUpdated = updatedTime
		} else {
			s.Users[id].LastUpdated, _ = time.Parse("2006-02-01", "0001-01-01")
		}
		s.Users[id].LastKnownLocation = data[id].LastLocation
		s.Users[id].Departments = []Department{{ID: data[id].DepartmentID, Name: data[id].DepartmentName}}
		if strings.HasPrefix(data[id].AccessLevelName, "Individual: ") {
			s.Users[id].AccessLevels = s.getExactAccessLevel(data[id])
		} else {
			s.Users[id].AccessLevels = []string{data[id].AccessLevelName}
		}
		s.Users[id].LocalID = data[id].LocalID
	}
	return nil
}

func (s *Site) getExactAccessLevel(data *userSQLQuery) []string {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s%s%d%s", s.BaseURL, "/api/v1/users/", data.ID, "/doorpermissionset"))
	if err != nil {
		log.Error().Err(err).Msg("Unable to get exact permissions")
		return []string{data.AccessLevelName}
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to get permissions")
		return []string{data.AccessLevelName}
	}
	permissions := Permission{}
	err = json.Unmarshal(bodyData, &permissions)
	if err != nil {
		return []string{data.AccessLevelName}
	}
	accessLevels := make([]string, 0)
	for index := range permissions.AccessLevels {
		accessLevels = append(accessLevels, s.AccessLevels[permissions.AccessLevels[index]].Name)
	}
	for index := range permissions.IndividualPermissions {
		accessLevels = append(accessLevels, s.AccessLevels[permissions.IndividualPermissions[index].ID].Name)
	}
	return accessLevels
}

func (s *Site) UpdateAccessLevels() error {
	levels, err := s.updateLevels()
	if err != nil {
		return err
	}
	areas, err := s.updateAreas()
	if err != nil {
		return err
	}
	s.AccessLevels = lo.Assign(levels, areas)
	return nil
}

func (s *Site) updateLevels() (map[int]*AccessLevel, error) {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/accesslevels"))
	if err != nil {
		return nil, err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to pull access levels")
		return nil, errors.New("unable to pull access levels")
	}
	accesslevels := make([]*AccessLevel, 50)
	err = json.Unmarshal(bodyData, &accesslevels)
	if err != nil {
		return nil, err
	}
	return lo.SliceToMap(accesslevels, func(item *AccessLevel) (int, *AccessLevel) {
		return item.ID, item
	}), nil
}

func (s *Site) updateAreas() (map[int]*AccessLevel, error) {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/accesslevels/areas"))
	if err != nil {
		return nil, err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to pull access levels")
		return nil, errors.New("unable to pull access levels")
	}
	accesslevels := make([]*Area, 50)
	err = json.Unmarshal(bodyData, &accesslevels)
	if err != nil {
		return nil, err
	}
	moo := lo.SliceToMap[*Area, int, *AccessLevel](accesslevels, func(item *Area) (int, *AccessLevel) {
		return item.ID, &AccessLevel{ID: item.ID, Name: "Idv: " + item.Name}
	})
	return moo, nil
}

func (s *Site) UpdateDoors() error {
	doors, err := s.getDoors()
	if err != nil {
		return err
	}
	doorStatus, err := s.getDoorStatus()
	if err != nil {
		return err
	}
	s.Doors = lo.SliceToMap(doors, func(item *Door) (uint64, *Door) {
		item.StatusFlag = doorStatus[int(item.ID)]
		item.AlarmStatus = item.StatusFlag & DoorStatus_IntruderAlarm
		return item.ID, item
	})
	return nil
}

func (s *Site) getDoors() ([]*Door, error) {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s/api/v1/doors", s.BaseURL))
	if err != nil {
		return nil, err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Bytes("Response", bodyData).Msg("Unable to pull doors")
		return nil, errors.New("unable to pull doors")
	}
	doorSlice := make([]*Door, 50)
	err = json.Unmarshal(bodyData, &doorSlice)
	if err != nil {
		return nil, err
	}
	return doorSlice, nil
}

func (s *Site) getDoorStatus() (map[int]int, error) {
	query := "SELECT Address, statusFlag FROM devices"
	resp, err := s.httpClient.Get(fmt.Sprintf("%s/api/v1/customquery/querydb?query=%s", s.BaseURL, url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unable to pull device status'")
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	sqlMap := make([]*deviceSQLQuery, 0)
	err = json.Unmarshal(bodyData, &sqlMap)
	if err != nil {
		return nil, err
	}
	return lo.Associate(sqlMap, func(item *deviceSQLQuery) (int, int) {
		return item.ID, item.Status
	}), nil
}

func (s *Site) UpdateDepartments() error {
	resp, err := s.httpClient.Get(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/departments"))
	if err != nil {
		return err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		log.Error().Bytes("Response", bodyData).Msg("Unable to update doors")
		return errors.New("unable to update doors")
	}
	departments := make([]*Department, 50)
	err = json.Unmarshal(bodyData, &departments)
	if err != nil {
		return err
	}
	s.Departments = lo.SliceToMap(departments, func(item *Department) (int, *Department) {
		return item.ID, item
	})
	return nil
}
