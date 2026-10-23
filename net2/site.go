package net2

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"github.com/greboid/net2/config"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	JsonContentType = "application/json"
)

//go:embed photoneeded.png
var photoneeded []byte

//go:embed blank.gif
var blank []byte

func (s *Site) Start() error {
	s.stateLock.Lock()
	s.Users = make(map[int]*User, 0)
	s.Departments = make(map[int]*Department)
	s.AccessLevels = make(map[int]*AccessLevel)
	s.Doors = make(map[uint64]*Door)
	s.stateLock.Unlock()
	localIDField, err := s.getLocalFieldName()
	if err != nil {
		return fmt.Errorf("unable to resolve local ID field %q: %w", s.localIDFieldName, err)
	}
	s.LocalIDField = localIDField
	if s.cron == nil {
		cron, err := gocron.NewScheduler(gocron.WithLocation(time.Now().Location()))
		if err != nil {
			return err
		}
		s.cron = cron
	}
	_, err = s.cron.NewJob(
		gocron.DurationJob(time.Minute),
		gocron.NewTask(func() {
			s.UpdateAll()
		}),
		gocron.WithTags("siteupdate"),
	)
	if err != nil {
		return err
	}
	s.cron.Start()
	return nil
}

func (s *Site) Stop() {
	if s.cron == nil {
		return
	}
	err := s.cron.Shutdown()
	if err != nil {
		log.Error().Err(err).Str("Site", s.Name).Msg("Error shutting down scheduler")
	}
}

// GetUnknownTokens returns any unknown token events recorded for the site;
// an empty (non-nil) slice when there are none so JSON renders as [].
func (s *Site) GetUnknownTokens() []Event {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	if s.UnknownTokens == nil {
		return []Event{}
	}
	return s.UnknownTokens
}

func (s *Site) GetUser(userID int) *User {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return s.Users[userID]
}

func (s *Site) GetUsers() map[int]*User {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return s.Users
}

func (s *Site) GetUserPicture(userID int) ([]byte, error) {
	resp, err := s.doGet(fmt.Sprintf("%s/api/v1/users/%d/image", s.BaseURL, userID))
	if err != nil {
		return nil, err
	}
	defer drainBody(resp)
	if resp.StatusCode == http.StatusNotFound {
		return photoneeded, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("user not found")
	}
	body, err := io.ReadAll(resp.Body)
	return body, nil
}

func (s *Site) GetUserPictureByLocalID(localID int) ([]byte, error) {
	var localIDString = strconv.Itoa(localID)
	s.stateLock.RLock()
	var userIDs = lo.Values(lo.PickBy(s.Users, func(_ int, user *User) bool {
		return user.LocalID == localIDString
	}))
	s.stateLock.RUnlock()
	if len(userIDs) != 1 {
		return nil, errors.New("user not found")
	}
	resp, err := s.doGet(fmt.Sprintf("%s/api/v1/users/%d/image", s.BaseURL, userIDs[0].ID))
	if err != nil {
		return nil, err
	}
	defer drainBody(resp)
	if resp.StatusCode == http.StatusNotFound {
		return photoneeded, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("user not found")
	}
	body, err := io.ReadAll(resp.Body)
	return body, nil
}

func (s *Site) GetBlankPicture() ([]byte, error) {
	return blank, nil
}

func (s *Site) GetUsersInDepartment(departmentMatch func(test Department) bool, userMatch func(test *User) bool) map[int]*User {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return lo.PickBy(s.Users, func(_ int, user *User) bool {
		return lo.CountBy(user.Departments, func(department Department) bool {
			return departmentMatch(department)
		}) > 0 && userMatch(user)
	})
}

func (s *Site) GetActiveUsersInDepartment(match func(test Department) bool) map[int]*User {
	today := time.Now()
	midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	return s.GetUsersInDepartment(match, func(test *User) bool {
		return test.Expiry.After(midnight) || test.Expiry.Equal(midnight) || test.Expiry == time.Time{}
	})
}

func (s *Site) GetTodaysActiveUsersInDepartment(match func(test Department) bool) map[int]*User {
	today := time.Now()
	midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	return s.GetUsersInDepartment(match, func(test *User) bool { return test.LastUpdated.After(midnight) })
}

func (s *Site) GetCancelledUsers() any {
	return s.GetUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CancelledDeptPrefix) }, func(test *User) bool { return true })
}

func (s *Site) GetActiveStaffToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.StaffDeptPrefix) })
}

func (s *Site) GetActiveStaff() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.StaffDeptPrefix) })
}

func (s *Site) GetStaff() map[int]*User {
	return s.GetUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.StaffDeptPrefix) }, func(test *User) bool { return true })
}

func (s *Site) GetActiveVisitorsToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.VisitorDeptPrefix) })
}

func (s *Site) GetActiveVisitors() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.VisitorDeptPrefix) })
}

func (s *Site) GetVisitors() map[int]*User {
	return s.GetUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.VisitorDeptPrefix) }, func(test *User) bool { return true })
}

func (s *Site) GetActiveContractorsToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.ContractorDeptPrefix) })
}

func (s *Site) GetActiveContractors() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.ContractorDeptPrefix) })
}

func (s *Site) GetContractors() map[int]*User {
	return s.GetUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.ContractorDeptPrefix) }, func(test *User) bool { return true })
}

func (s *Site) GetActiveCleanersToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CleanerDeptPrefix) })
}

func (s *Site) GetActiveCleaners() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CleanerDeptPrefix) })
}

func (s *Site) GetCleaners() map[int]*User {
	return s.GetUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CleanerDeptPrefix) }, func(test *User) bool { return true })
}

func (s *Site) GetActiveCustomersToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CustomerDeptPrefix) })
}

func (s *Site) GetActiveCustomers() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CustomerDeptPrefix) })
}

func (s *Site) GetCustomers() map[int]*User {
	return s.GetUsersInDepartment(func(test Department) bool { return strings.HasPrefix(test.Name, s.config.CustomerDeptPrefix) }, func(test *User) bool { return true })
}

func (s *Site) GetActiveUsersToday() map[int]*User {
	return s.GetTodaysActiveUsersInDepartment(func(test Department) bool { return true })
}

func (s *Site) GetActiveUsers() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool { return true })
}

func (s *Site) GetActiveNonStaff() map[int]*User {
	return s.GetActiveUsersInDepartment(func(test Department) bool {
		return !strings.HasPrefix(test.Name, s.config.StaffDeptPrefix)
	})
}

func (s *Site) GetDoors() map[uint64]*Door {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return s.Doors
}

func (s *Site) GetMonitoredDoors() map[uint64]*Door {
	s.stateLock.RLock()
	doors := lo.PickBy(s.Doors, func(_ uint64, door *Door) bool {
		return lo.CountBy(s.config.MonitoredDoors, func(monitoredDoor config.MonitoredDoor) bool {
			return uint64(monitoredDoor.ID) == door.ID
		}) > 0
	})
	s.stateLock.RUnlock()
	lo.ForEach(s.config.MonitoredDoors, func(item config.MonitoredDoor, index int) {
		if val, ok := doors[uint64(item.ID)]; ok {
			monitoredDoor := *val
			monitoredDoor.Name = item.Name
			doors[uint64(item.ID)] = &monitoredDoor
		}
	})
	return doors
}

func (s *Site) GetOpenableDoors() map[string][]config.DoorSequence {
	return lo.SliceToMap(s.config.OpenableDoors, func(item config.OpenableDoor) (string, []config.DoorSequence) {
		return item.Name, item.Sequence
	})
}

func (s *Site) GetDoor(doorID uint64) *Door {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return s.Doors[doorID]
}

func (s *Site) OpenDoor(doorID uint64) error {
	jsonBytes, _ := json.Marshal(map[string]uint64{"doorId": doorID})
	if !s.hasDoor(doorID) {
		return errors.New("invalid door")
	}
	resp, err := s.doPost(fmt.Sprintf("%s/api/v1/commands/door/open", s.BaseURL), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != 200 {
		log.Error().Int("Status", resp.StatusCode).Msg("Unable to open door")
		return errors.New("unable to open door")
	}
	return nil
}

func (s *Site) OpenDoorWithRelay(doorID uint64, secondRelay bool) error {
	var relay string
	if secondRelay {
		relay = "Relay2"
	} else {
		relay = "Relay1"
	}
	jsonBytes, _ := json.Marshal(map[string]any{
		"doorId": doorID,
		"RelayFunction": map[string]any{
			"RelayId":       relay,
			"RelayAction":   "TimedOpen",
			"RelayOpenTime": 1000,
		},
	})
	if !s.hasDoor(doorID) {
		return errors.New("invalid door")
	}
	resp, err := s.doPost(fmt.Sprintf("%s/api/v1/commands/door/control", s.BaseURL), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != 200 {
		log.Error().Int("Status", resp.StatusCode).Msg("Unable to open door")
		return errors.New("unable to open door")
	}
	return nil
}

func (s *Site) CloseDoor(doorID uint64) error {
	jsonBytes, _ := json.Marshal(map[string]uint64{"doorId": doorID})
	if !s.hasDoor(doorID) {
		return errors.New("invalid door")
	}
	resp, err := s.doPost(fmt.Sprintf("%s/api/v1/commands/door/close", s.BaseURL), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != 200 {
		log.Error().Int("Status", resp.StatusCode).Msg("Unable to close door")
		return errors.New("unable to close door")
	}
	return nil
}

func (s *Site) GetAccessLevels() map[int]*AccessLevel {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return s.AccessLevels
}

func (s *Site) GetDepartments() map[int]*Department {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return s.Departments
}

// UpToDate reports whether the last successful full update was within the
// last 3 minutes.
func (s *Site) UpToDate() bool {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	return !s.LastPolled.Before(time.Now().Add(-3 * time.Minute))
}

// hasDoor reports whether the given door is known to the site.
func (s *Site) hasDoor(doorID uint64) bool {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	_, ok := s.Doors[doorID]
	return ok
}

// getClient returns the current HTTP client; it is safe for concurrent use
// because the client can be replaced on authentication failures.
func (s *Site) getClient() *http.Client {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	return s.httpClient
}

// drainBody discards and closes a response body so the underlying
// connection can be reused (and is not leaked on early returns).
func drainBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func (s *Site) doRequest(method string, url string, body io.Reader) (*http.Response, error) {
	// Buffer the body up front so it can be replayed if the request has to
	// be retried after re-authenticating; a consumed reader would otherwise
	// send an empty body on the retry.
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}
	var req *http.Request
	var resp *http.Response
	for tryReauth := 2; tryReauth > 0; tryReauth-- {
		if bodyBytes != nil {
			req, err = http.NewRequest(method, url, bytes.NewReader(bodyBytes))
		} else {
			req, err = http.NewRequest(method, url, nil)
		}
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", JsonContentType)
		resp, err = s.getClient().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized {
			break
		}
		drainBody(resp)
		s.clientMu.Lock()
		s.httpClient = getHttpClient(s.clientID, s.config.Username, s.config.Password, s.BaseURL, s.config.VerifyTLS)
		s.clientMu.Unlock()
	}
	return resp, err
}

func (s *Site) doPost(url string, body io.Reader) (*http.Response, error) {
	return s.doRequest(http.MethodPost, url, body)
}

func (s *Site) doPut(url string, body io.Reader) (*http.Response, error) {
	return s.doRequest(http.MethodPut, url, body)
}

func (s *Site) doGet(url string) (*http.Response, error) {
	return s.doRequest(http.MethodGet, url, nil)
}

func (s *Site) ResetAntiPassback(userID int) error {
	jsonBytes, _ := json.Marshal(map[string]int{"userId": userID})
	resp, err := s.doPost(fmt.Sprintf("%s/api/v1/commands/antipassback/reset", s.BaseURL), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != 200 {
		return errors.New("unable to reset anti passback")
	}
	return nil
}

func (s *Site) ActivateUser(userID int) error {
	return s.UpdateUserInfo(userID, map[string]any{
		"ExpiryDate": GetTomorrow(),
	})
}

func (s *Site) DeactivateUser(userID int) error {
	return s.UpdateUserInfo(userID, map[string]any{
		"ExpiryDate": GetYesterday(),
	})
}

func (s *Site) UpdateUserInfo(userID int, info map[string]any) error {
	info["Id"] = userID
	if _, ok := info["ExpiryDate"]; !ok {
		s.stateLock.RLock()
		user := s.Users[userID]
		s.stateLock.RUnlock()
		if user == nil {
			return errors.New("invalid user")
		}
		info["ExpiryDate"] = user.Expiry
	}
	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return err
	}
	resp, err := s.doPut(fmt.Sprintf("%s/api/v1/users/%d", s.BaseURL, userID), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != 200 {
		s.logger.Error().Int("Status", resp.StatusCode).Msg("Unable to update user")
		return errors.New("unable to update user info")
	}
	return s.UpdateUser(userID)
}

func (s *Site) ChangeUserDepartment(userID, departmentID int) error {
	s.stateLock.RLock()
	newDepartment, ok := s.Departments[departmentID]
	s.stateLock.RUnlock()
	if !ok {
		return fmt.Errorf("department not found: %d", departmentID)
	}
	jsonBytes, err := json.Marshal(newDepartment)
	if err != nil {
		return err
	}
	resp, err := s.doPut(fmt.Sprintf("%s/api/v1/users/%d/departments", s.BaseURL, userID), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != 204 {
		s.logger.Error().Int("Status", resp.StatusCode).Msg("Unable to update user department")
		return errors.New("unable to update user department")
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
	resp, err := s.doPut(fmt.Sprintf("%s/api/v1/users/%d/doorpermissionset", s.BaseURL, userID), bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		s.logger.Error().Int("Status", resp.StatusCode).Msg("Unable to update user access level")
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
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	for id := range s.AccessLevels {
		if s.AccessLevels[id].Name == levelName {
			return s.AccessLevels[id].ID
		}
	}
	return -1
}

func (s *Site) RemoveUserAccessLevel(userID int, accesslevel int) error {
	s.stateLock.RLock()
	user, ok := s.Users[userID]
	s.stateLock.RUnlock()
	if !ok {
		return errors.New("invalid user")
	}
	existingLevelNames := user.AccessLevels
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
	s.stateLock.RLock()
	user, ok := s.Users[userID]
	s.stateLock.RUnlock()
	if !ok {
		return errors.New("invalid user")
	}
	existingLevelNames := user.AccessLevels
	newLevels := make([]int, 0)
	for index := range existingLevelNames {
		newLevels = append(newLevels, s.getAccessLevelIDByName(existingLevelNames[index]))
	}
	newLevels = append(newLevels, accesslevel)
	return s.UpdateUserAccessLevels(userID, newLevels)
}

func (s *Site) SequenceDoor(items ...DoorSequenceItem) {
	for _, value := range items {
		err := s.OpenDoor(value.Door)
		if err != nil {
			log.Error().Err(err).Interface("Doors", items).Msg("Unable to open door in sequence")
		}
		time.Sleep(value.Time)
	}
}

func (s *Site) UpdateUserNameAndExpiryAndAccessLevel(userid int, firstname string, surname string, expiry time.Time, level int) error {
	err := s.UpdateUserInfo(userid, map[string]any{
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
	log.Debug().Str("Site", s.Name).Msg("Starting full update")
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	var err error
	start := time.Now()
	complete := true
	err = s.UpdateAccessLevels()
	if err != nil {
		complete = false
		log.Error().Err(err).Str("Site", s.Name).Msg("Error updating access levels")
	} else {
		log.Debug().Str("Site", s.Name).Msg("Updated access levels")
	}
	err = s.UpdateDoors()
	if err != nil {
		complete = false
		log.Error().Err(err).Str("Site", s.Name).Msg("Error updating doors")
	} else {
		log.Debug().Str("Site", s.Name).Msg("Updated doors")
	}
	err = s.UpdateDepartments()
	if err != nil {
		complete = false
		log.Error().Err(err).Str("Site", s.Name).Msg("Error updating departments")
	} else {
		log.Debug().Str("Site", s.Name).Msg("Updated departments")
	}
	err = s.UpdateUsers()
	if err != nil {
		complete = false
		log.Error().Err(err).Str("Site", s.Name).Msg("Error updating users")
	} else {
		log.Debug().Str("Site", s.Name).Msg("Updated users")
	}
	total := time.Now().Sub(start).Milliseconds()
	if !complete {
		log.Info().Str("Site", s.Name).Int64("Total (ms)", total).Msg("Full update Failed")
	} else {
		log.Debug().Str("Site", s.Name).Int64("Total (ms)", total).Msg("Full update completed")
		s.stateLock.Lock()
		s.LastPolled = time.Now()
		s.stateLock.Unlock()
	}
}

// getLocalFieldName resolves the configured local ID field name to the
// Net2 query-field identifier used in user queries; without it every user
// query would be invalid SQL, so callers must treat failure as fatal.
func (s *Site) getLocalFieldName() (string, error) {
	if s.localIDFieldName == "" {
		return "", errors.New("local ID field name is not configured")
	}
	resp, err := s.doGet(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/users/customfieldnames"))
	if err != nil {
		return "", err
	}
	defer drainBody(resp)
	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to get custom fields")
		return "", fmt.Errorf("custom field names request failed with status %d", resp.StatusCode)
	}
	fields := []*CustomFieldDefinition{}
	if err := json.Unmarshal(bodyData, &fields); err != nil {
		return "", fmt.Errorf("unable to unmarshal custom fields: %w", err)
	}
	for index := range fields {
		if fields[index].Name == s.localIDFieldName {
			if fields[index].ID == 1 || fields[index].ID == 2 {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "100"), nil
			} else if fields[index].ID == 6 || fields[index].ID == 7 {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "60"), nil
			} else if fields[index].ID == 13 {
				return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "Memo"), nil
			}
			return fmt.Sprintf("%s%d_%s", "Field", fields[index].ID, "50"), nil
		}
	}
	return "", fmt.Errorf("local ID field %q not found in Net2 custom fields", s.localIDFieldName)
}

func (s *Site) UpdateUser(userID int) error {
	return s.updateUsersWithData(fmt.Sprintf("SELECT *, %s as LocalID FROM UsersEx WHERE userID=%d AND Active=1", s.LocalIDField, userID))
}

func (s *Site) UpdateUsers() error {
	return s.updateUsersWithData(fmt.Sprintf("SELECT *, %s as LocalID FROM UsersEx WHERE Active=1", s.LocalIDField))
}

func (s *Site) updateUsersWithData(query string) error {
	resp, err := s.doGet(fmt.Sprintf("%s/api/v1/customquery/querydb?query=%s", s.BaseURL, url.QueryEscape(query)))
	if err != nil {
		return err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to pull users")
		return errors.New("unable to update user info")
	}
	data := []*userSQLQuery{}
	err = json.Unmarshal(bodyData, &data)
	if err != nil {
		return err
	}
	updates := make(map[int]*User, len(data))
	for id := range data {
		user := &User{
			GUID:              data[id].UserGUID,
			ID:                data[id].ID,
			FirstName:         data[id].Firstname,
			Surname:           data[id].Surname,
			PIN:               data[id].PIN,
			Departments:       []Department{{ID: data[id].DepartmentID, Name: data[id].DepartmentName}},
			LastKnownLocation: data[id].LastLocation,
		}
		if updatedTime, err := time.ParseInLocation("2006-01-02T15:04:05", data[id].ActivateDate, time.Local); err == nil {
			user.Activated = updatedTime
		} else {
			user.Activated, _ = time.Parse("2006-01-02", "0001-01-01")
		}
		if updatedTime, err := time.ParseInLocation("2006-01-02T15:04:05", data[id].ExpiryDate, time.Local); err == nil {
			user.Expiry = updatedTime
		} else {
			user.Expiry, _ = time.Parse("2006-01-02", "0001-01-01")
		}
		if updatedTime, err := time.ParseInLocation("2006-01-02T15:04:05", data[id].LastAccessTime, time.Local); err == nil {
			user.LastUpdated = updatedTime
		} else {
			user.LastUpdated, _ = time.Parse("2006-01-02", "0001-01-01")
		}
		if strings.HasPrefix(data[id].AccessLevelName, "Individual: ") {
			user.AccessLevels = s.getExactAccessLevel(data[id])
		} else {
			user.AccessLevels = []string{data[id].AccessLevelName}
		}
		user.LocalID = data[id].LocalID
		updates[user.ID] = user
	}
	// Publish the fresh map wholesale: entries that no longer appear in
	// the query (e.g. users deactivated or removed on the Net2 side) must
	// not linger in the cache.  Readers may hold on to the previous map,
	// which stays valid because published maps are never mutated.
	s.stateLock.Lock()
	s.Users = updates
	s.stateLock.Unlock()
	return nil
}

func (s *Site) getExactAccessLevel(data *userSQLQuery) []string {
	resp, err := s.doGet(fmt.Sprintf("%s%s%d%s", s.BaseURL, "/api/v1/users/", data.ID, "/doorpermissionset"))
	if err != nil {
		log.Error().Err(err).Msg("Unable to get exact permissions")
		return []string{data.AccessLevelName}
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to get permissions")
		return []string{data.AccessLevelName}
	}
	permissions := Permission{}
	err = json.Unmarshal(bodyData, &permissions)
	if err != nil {
		return []string{data.AccessLevelName}
	}
	accessLevels := make([]string, 0)
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	for index := range permissions.AccessLevels {
		if s.AccessLevels[permissions.AccessLevels[index]] != nil {
			accessLevels = append(accessLevels, s.AccessLevels[permissions.AccessLevels[index]].Name)
		} else {
			log.Debug().Str("Site", s.Name).Interface("Access Level ID", permissions.AccessLevels[index]).Interface("User", data.Firstname+" "+data.Surname).Msg("Discarding invalid access level")
		}
	}
	for index := range permissions.IndividualPermissions {
		if s.AccessLevels[permissions.IndividualPermissions[index].ID] != nil {
			accessLevels = append(accessLevels, s.AccessLevels[permissions.IndividualPermissions[index].ID].Name)
		} else {
			log.Debug().Str("Site", s.Name).Interface("Idv Perm ID", permissions.IndividualPermissions[index].ID).Interface("User", data.Firstname+" "+data.Surname).Msg("Discarding invalid access level")
		}
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
	s.stateLock.Lock()
	s.AccessLevels = lo.Assign(levels, areas)
	s.stateLock.Unlock()
	return nil
}

func (s *Site) updateLevels() (map[int]*AccessLevel, error) {
	resp, err := s.doGet(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/accesslevels"))
	if err != nil {
		return nil, err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to pull access levels")
		return nil, errors.New("unable to pull access levels")
	}
	accesslevels := []*AccessLevel{}
	err = json.Unmarshal(bodyData, &accesslevels)
	if err != nil {
		return nil, err
	}
	return lo.SliceToMap(accesslevels, func(item *AccessLevel) (int, *AccessLevel) {
		return item.ID, item
	}), nil
}

func (s *Site) updateAreas() (map[int]*AccessLevel, error) {
	resp, err := s.doGet(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/accesslevels/areas"))
	if err != nil {
		return nil, err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to pull access levels")
		return nil, errors.New("unable to pull access levels")
	}
	accesslevels := []*Area{}
	err = json.Unmarshal(bodyData, &accesslevels)
	if err != nil {
		return nil, err
	}
	moo := lo.SliceToMap[*Area, int, *AccessLevel](accesslevels, func(item *Area) (int, *AccessLevel) {
		return 10000 + item.ID, &AccessLevel{ID: 10000 + item.ID, Name: "Idv: " + item.Name}
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
	doorMap := lo.SliceToMap(doors, func(item *Door) (uint64, *Door) {
		item.StatusFlag = doorStatus[int(item.ID)]
		item.AlarmStatus = item.StatusFlag & DoorStatus_IntruderAlarm
		return item.ID, item
	})
	lo.ForEach(s.config.MonitoredDoors, func(item config.MonitoredDoor, index int) {
		if val, ok := doorMap[uint64(item.ID)]; ok {
			val.AlarmZone = item.Zone
		}
	})
	s.stateLock.Lock()
	s.Doors = doorMap
	s.stateLock.Unlock()
	return nil
}

func (s *Site) getDoors() ([]*Door, error) {
	resp, err := s.doGet(fmt.Sprintf("%s/api/v1/doors", s.BaseURL))
	if err != nil {
		return nil, err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to pull doors")
		return nil, errors.New("unable to pull doors")
	}
	doorSlice := []*Door{}
	err = json.Unmarshal(bodyData, &doorSlice)
	if err != nil {
		return nil, err
	}
	return doorSlice, nil
}

func (s *Site) getDoorStatus() (map[int]int, error) {
	query := "SELECT Address, statusFlag FROM devices"
	resp, err := s.doGet(fmt.Sprintf("%s/api/v1/customquery/querydb?query=%s", s.BaseURL, url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unable to pull device status'")
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	sqlMap := []*deviceSQLQuery{}
	err = json.Unmarshal(bodyData, &sqlMap)
	if err != nil {
		return nil, err
	}
	return lo.Associate(sqlMap, func(item *deviceSQLQuery) (int, int) {
		return item.ID, item.Status
	}), nil
}

func (s *Site) UpdateDepartments() error {
	resp, err := s.doGet(fmt.Sprintf("%s%s", s.BaseURL, "/api/v1/departments"))
	if err != nil {
		return err
	}
	bodyData, _ := io.ReadAll(resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		s.logger.Error().Int("Status", resp.StatusCode).Str("URL", resp.Request.URL.String()).Msg("Unable to update doors")
		return errors.New("unable to update doors")
	}
	departments := []*Department{}
	err = json.Unmarshal(bodyData, &departments)
	if err != nil {
		return err
	}
	departmentMap := lo.SliceToMap(departments, func(item *Department) (int, *Department) {
		return item.ID, item
	})
	s.stateLock.Lock()
	s.Departments = departmentMap
	s.stateLock.Unlock()
	return nil
}

func GetTomorrow() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 23, 59, 0, 0, time.Local)
}

func GetYesterday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 0, 0, time.Local)
}
