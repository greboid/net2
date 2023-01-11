package api

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"net/http"
	"net2/net2"
	"sort"
	"strconv"
	"time"
)

type Web struct {
	e      *echo.Echo
	sites  *net2.SiteManager
	quit   chan bool
	logger *zerolog.Logger
}

func NewWeb(sites *net2.SiteManager, logger *zerolog.Logger) *Web {
	web := &Web{
		sites:  sites,
		quit:   make(chan bool),
		logger: logger,
	}
	web.init()
	return web
}

func (web *Web) Quit() {
	web.quit <- true
}

func (web *Web) init() {
	web.e = echo.New()
	web.e.HidePort = true
	web.e.HideBanner = true
	web.e.Debug = true
	web.e.Use(middleware.RemoveTrailingSlashWithConfig(middleware.TrailingSlashConfig{
		RedirectCode: http.StatusTemporaryRedirect,
	}))
	web.e.Use(middleware.CORS())

	web.e.GET("/favicon.ico", func(c echo.Context) error {
		return c.NoContent(200)
	})
	web.e.Static("", "./templates")

	api := web.e.Group("/api/v1")
	api.GET("", web.getAPIIndex)
	api.GET("/triggerUpdate", web.update)
	api.GET("/updateNow", web.updateNow)

	//Site functions
	api.GET("/sites", web.getSites)
	api.GET("/sites/:siteid", web.getSite)
	api.GET("/sites/:siteid/uptodate", web.getUpToDate)
	api.GET("/sites/:siteid/unknownTokens", web.getUnknownTokens)

	//Access level functions
	api.GET("/sites/:siteid/accesslevels", web.getAccessLevels)

	//Department functions
	api.GET("/sites/:siteid/departments", web.getDepartments)

	//Door functions
	api.GET("/sites/:siteid/doors", web.getDoors)
	api.POST("/sites/:siteid/doors/sequence", web.sequenceDoors)

	//Individual door functions
	api.GET("/sites/:siteid/doors/:doorid", web.getDoor)
	api.POST("/sites/:siteid/doors/:doorid/open", web.openDoor)
	api.POST("/sites/:siteid/doors/:doorid/close", web.closeDoor)

	//User functions
	api.GET("/sites/:siteid/users", web.getUsers)
	api.GET("/sites/:siteid/users/active", web.getActiveUsers)
	api.GET("/sites/:siteid/users/activetoday", web.getActiveUsersToday)
	api.GET("/sites/:siteid/users/activestaff", web.getActiveStaff)
	api.GET("/sites/:siteid/users/activestafftoday", web.getActiveStaffToday)
	api.GET("/sites/:siteid/users/activevisitors", web.getActiveVisitors)
	api.GET("/sites/:siteid/users/activevisitorstoday", web.getActiveVisitors)

	//Individual user functions
	api.GET("/sites/:siteid/users/:userid", web.getUser)
	api.GET("/sites/:siteid/users/:userid/picture", web.getUserPicture)
	api.POST("/sites/:siteid/users/:userid/resetantipassback", web.resetAntiPassback)
	api.POST("/sites/:siteid/users/:userid/activate", web.activateUser)
	api.POST("/sites/:siteid/users/:userid/deactivate", web.deactivateUser)
	api.POST("/sites/:siteid/users/:userid/activateAndUpdate", web.activateAndUpdate)
	api.POST("/sites/:siteid/users/:userid/deactivateAndUpdate", web.deactivateAndUpdate)
	api.POST("/sites/:siteid/users/:userid/extendexpiry", web.extendExpiry)
	api.POST("/sites/:siteid/users/:userid/addaccesslevel", web.addAccessLevel)
	api.POST("/sites/:siteid/users/:userid/removeaccesslevel", web.removeAccessLevel)
	api.POST("/sites/:siteid/users/:userid/changedepartment", web.changeDepartment)
	api.POST("/sites/:siteid/users/:userid/setaccesslevel", web.setAccessLevel)
}

func (web *Web) getAPIIndex(c echo.Context) error {
	routes := make([]string, 0)
	for _, route := range web.e.Routes() {
		routes = append(routes, fmt.Sprintf("%s (%s)\r\n", route.Path, route.Method))
	}
	sort.Strings(routes)
	output := "Updates once per minute.\r\n"
	output += "Available routes:\r\n"
	for _, route := range routes {
		output += route
	}
	return c.String(http.StatusOK, output)
}

func (web *Web) Start(port int) {
	web.logger.Info().Int("Port", port).Msg("Starting webserver")
	go func() {
		err := web.e.Start(fmt.Sprintf(":%d", port))
		if err != nil {
			web.logger.Error().Err(err).Msg("Error shutting down server")
		}
	}()
	<-web.quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := web.e.Shutdown(ctx); err != nil {
		web.e.Logger.Fatal(err)
	}
}

func (web *Web) getUsers(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetUsers())
}

func (web *Web) getUser(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetUser(userID))
}

func (web *Web) getUserPicture(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	picture, err := site.GetUserPicture(userID)
	if err != nil {
		return c.String(http.StatusNotFound, "error getting picture")
	}
	return c.Blob(http.StatusOK, "image/jpeg", picture)
}

func (web *Web) getSites(c echo.Context) error {
	return c.JSON(http.StatusOK, web.sites.GetSites())
}

func (web *Web) getSite(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site)
}

func (web *Web) getActiveStaffToday(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetActiveStaffToday())
}

func (web *Web) getActiveStaff(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetActiveStaff())
}

func (web *Web) getActiveVisitors(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetActiveVisitors())
}

func (web *Web) getActiveUsersToday(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetActiveUsersToday())
}

func (web *Web) getActiveUsers(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetActiveUsers())
}

func (web *Web) getDoors(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetDoors())
}

func (web *Web) getDoor(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	doorID, err := strconv.Atoi(c.Param("doorid"))
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("door id needs to be numeric: %s", err.Error()))
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetDoor(uint64(doorID)))
}

func (web *Web) openDoor(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	doorID, err := strconv.Atoi(c.Param("doorid"))
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("door id needs to be numeric: %s", err.Error()))
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	err = site.OpenDoor(uint64(doorID))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error opening door")
	}
	return c.String(http.StatusOK, "Door opened")
}

func (web *Web) closeDoor(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	doorID, err := strconv.Atoi(c.Param("doorid"))
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("door id needs to be numeric: %s", err.Error()))
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	err = site.CloseDoor(uint64(doorID))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error closing door")
	}
	return c.String(http.StatusOK, "Door closed")
}

func (web *Web) resetAntiPassback(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("user id needs to be numeric: %s", err.Error()))
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	err = site.ResetAntiPassback(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error resetting anti passback")
	}
	return c.String(http.StatusOK, "Anti passback reset")
}

func (web *Web) getAccessLevels(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetAccessLevels())
}

func (web *Web) getDepartments(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.GetDepartments())
}

func (web *Web) activateUser(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	err = site.ActivateUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error activating user: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User activated")
}

func (web *Web) deactivateUser(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	err = site.DeactivateUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error deactivating user: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User deactivated")
}

func (web *Web) activateAndUpdate(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	data := make(map[string]interface{})
	err = c.Bind(&data)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("bad data: %s", err.Error()))
	}
	firstNameRaw, ok := data["FirstName"]
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: FirstName must be present")
	}
	firstName, ok := firstNameRaw.(string)
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: FirstName must be string")
	}
	lastNameRaw, ok := data["LastName"]
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: LastName must be present")
	}
	lastName, ok := lastNameRaw.(string)
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: LastName must be string")
	}
	accessLevelRaw, _ := data["AccessLevel"]
	if !ok {
		accessLevelRaw = nil
	}
	accessLevel := -1
	accessLevelFloat, ok := accessLevelRaw.(float64)
	if ok {
		accessLevel = int(accessLevelFloat)
	} else {
		accessLevelInt, ok := accessLevelRaw.(int)
		if ok {
			accessLevel = accessLevelInt
		}
	}
	if accessLevel == -1 {
		return c.String(http.StatusBadRequest, "Data error: AccessLevel must be int")
	}
	err = site.UpdateUserNameAndExpiryAndAccessLevel(
		userID,
		firstName,
		lastName,
		GetTomorrow(),
		accessLevel,
	)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error activating user: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User activated")
}

func (web *Web) deactivateAndUpdate(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	data := make(map[string]interface{})
	err = c.Bind(&data)
	if err != nil {
		return c.String(http.StatusBadRequest, fmt.Sprintf("bad data: %s", err.Error()))
	}
	firstNameRaw, ok := data["FirstName"]
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: FirstName must be present")
	}
	firstName, ok := firstNameRaw.(string)
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: FirstName must be string")
	}
	lastNameRaw, ok := data["LastName"]
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: LastName must be present")
	}
	lastName, ok := lastNameRaw.(string)
	if !ok {
		return c.String(http.StatusBadRequest, "Data error: LastName must be string")
	}
	accessLevelRaw, _ := data["AccessLevel"]
	if !ok {
		accessLevelRaw = nil
	}
	accessLevel := -1
	accessLevelFloat, ok := accessLevelRaw.(float64)
	if ok {
		accessLevel = int(accessLevelFloat)
	} else {
		accessLevelInt, ok := accessLevelRaw.(int)
		if ok {
			accessLevel = accessLevelInt
		}
	}
	if accessLevel == -1 {
		return c.String(http.StatusBadRequest, "Data error: AccessLevel must be int")
	}
	err = site.UpdateUserNameAndExpiryAndAccessLevel(
		userID,
		firstName,
		lastName,
		GetYesterday(),
		accessLevel,
	)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error deactivating user: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User deactivated")
}

func (web *Web) extendExpiry(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	err = site.UpdateUserInfo(userID, map[string]interface{}{"ExpiryDate": GetTomorrow()})
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error extending user expiry: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User expiry extended")
}

func (web *Web) update(c echo.Context) error {
	go func() {
		web.sites.UpdateAll()
	}()
	return c.JSON(http.StatusOK, "Update triggered")
}

func (web *Web) updateNow(c echo.Context) error {
	web.sites.UpdateAll()
	return c.JSON(http.StatusOK, "Update complete")
}

func (web *Web) sequenceDoors(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	door1, err := strconv.ParseUint(c.QueryParam("d1"), 0, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "d1 needs to be numeric")
	}
	time1, err := time.ParseDuration(c.QueryParam("t1"))
	if err != nil {
		return c.String(http.StatusBadRequest, "t1 needs to be duration")
	}
	door2, err := strconv.ParseUint(c.QueryParam("d2"), 0, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "d2 needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	go func() {
		site.SequenceDoor(net2.DoorSequenceItem{Door: door1, Time: time1}, net2.DoorSequenceItem{Door: door2})
	}()
	return c.JSON(http.StatusOK, "Sequence triggered")
}

func (web *Web) addAccessLevel(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	level, err := strconv.Atoi(c.QueryParam("level"))
	if err != nil {
		return c.String(http.StatusBadRequest, "level needs to be numeric")
	}
	err = site.AddUserAccessLevel(userID, level)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error extending user expiry: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User access level added")
}

func (web *Web) removeAccessLevel(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	level, err := strconv.Atoi(c.QueryParam("level"))
	if err != nil {
		return c.String(http.StatusBadRequest, "level needs to be numeric")
	}
	err = site.RemoveUserAccessLevel(userID, level)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error extending user expiry: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User access level removed")
}

func (web *Web) setAccessLevel(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	level, err := strconv.Atoi(c.QueryParam("level"))
	if err != nil {
		return c.String(http.StatusBadRequest, "level needs to be numeric")
	}
	err = site.SetUserAccessLevel(userID, level)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error extending user expiry: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User access level updated")
}

func (web *Web) getUpToDate(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, !site.LastPolled.Before(time.Now().Add(-1*3*time.Minute)))
}

func (web *Web) getUnknownTokens(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	return c.JSON(http.StatusOK, site.UnknownTokens)
}

func (web *Web) changeDepartment(c echo.Context) error {
	siteID, err := strconv.Atoi(c.Param("siteid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "site id needs to be numeric")
	}
	userID, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		return c.String(http.StatusBadRequest, "user id needs to be numeric")
	}
	site := web.sites.GetSite(siteID)
	if site == nil {
		return c.String(http.StatusNotFound, "site id not found")
	}
	department, err := strconv.Atoi(c.QueryParam("department"))
	if err != nil {
		return c.String(http.StatusBadRequest, "department needs to be numeric")
	}
	err = site.ChangeUserDepartment(userID, department)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error extending changing user department: %s", err.Error()))
	}
	return c.JSON(http.StatusOK, "User department changed")
}

func GetTomorrow() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 23, 59, 0, 0, time.Local)
}

func GetYesterday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 0, 0, time.Local)
}
