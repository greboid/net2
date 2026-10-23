package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/greboid/net2/net2"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type MessageResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type UpdateUserData struct {
	FirstName   string `json:"FirstName"`
	LastName    string `json:"LastName"`
	AccessLevel int    `json:"AccessLevel"`
}

func (d *UpdateUserData) Bind(_ *http.Request) error {
	if d.FirstName == "" {
		return errors.New("missing required field FirstName")
	}
	if d.LastName == "" {
		return errors.New("missing required field LastName")
	}
	return nil
}

type SequenceDoorData struct {
	Door string `json:"door"`
	Time string `json:"time"`
}

func (d *SequenceDoorData) Bind(_ *http.Request) error {
	return nil
}

// Request-scoped values populated by the validation middleware; handlers
// read them through the accessors below instead of re-parsing URL params.
type contextKey int

const (
	siteContextKey contextKey = iota
	userIDContextKey
	doorIDContextKey
	doorNameContextKey
	departmentNameContextKey
)

func withValue[T any](r *http.Request, key contextKey, value T) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), key, value))
}

func (s *Server) site(r *http.Request) *net2.Site {
	return r.Context().Value(siteContextKey).(*net2.Site)
}

func (s *Server) userID(r *http.Request) int {
	return r.Context().Value(userIDContextKey).(int)
}

func (s *Server) doorID(r *http.Request) uint64 {
	return r.Context().Value(doorIDContextKey).(uint64)
}

func (s *Server) doorName(r *http.Request) string {
	return r.Context().Value(doorNameContextKey).(string)
}

func (s *Server) departmentName(r *http.Request) string {
	return r.Context().Value(departmentNameContextKey).(string)
}

func (s *Server) GetRoutes() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, MessageResponse{Error: "Resource not found"})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusMethodNotAllowed)
		render.JSON(w, r, MessageResponse{Error: "Method not allowed"})
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", s.Index)
		r.Route("/update", func(r chi.Router) {
			r.Get("/now", s.updateNow)
			r.Get("/trigger", s.update)
		})
		r.Route("/sites", func(r chi.Router) {
			// The timeout is applied to the site routes only: the update
			// endpoints can legitimately run for longer than 10 seconds.
			r.Use(middleware.Timeout(10 * time.Second))
			r.Get("/", s.getSites)
			r.With(s.validateSiteID).Route("/{siteID:[0-9]+}", func(r chi.Router) {
				r.Get("/", s.getSite)
				r.Get("/uptodate", s.getUpToDate)
				r.Get("/unknownTokens", s.getUnknownTokens)
				r.Route("/accesslevels", func(r chi.Router) {
					r.Get("/", s.getAccessLevels)
				})
				r.Route("/departments", func(r chi.Router) {
					r.Get("/", s.getDepartments)
					r.With(s.validateDepartmentName).Route("/{departmentName}", func(r chi.Router) {
						r.Post("/activate", s.activateDepartmentUsers)
					})
				})
				r.Route("/doors", func(r chi.Router) {
					r.Get("/", s.getDoors)
					r.Get("/monitored", s.getMonitoredDoors)
					r.Get("/openable", s.getOpenableDoors)
					r.With(s.validateOpenableDoor).Route("/openable/{doorName}", func(r chi.Router) {
						r.Post("/open", s.openOpenableDoor)
					})
					r.Post("/sequence", s.sequenceDoors)
					r.With(s.validateDoorID).Route("/{doorID:[0-9]+}", func(r chi.Router) {
						r.Get("/", s.getDoor)
						r.Post("/open", s.openDoor)
						r.Post("/relay1", s.relay1)
						r.Post("/relay2", s.relay2)
						r.Post("/close", s.closeDoor)
					})
				})
				r.Route("/users", func(r chi.Router) {
					r.Get("/", s.getUsers)
					r.Get("/active", s.getActiveUsers)
					r.Get("/activetoday", s.getActiveUsersToday)
					r.Get("/activestaff", s.getActiveStaff)
					r.Get("/activestafftoday", s.getActiveStaffToday)
					r.Get("/activevisitors", s.getActiveVisitors)
					r.Get("/activevisitorstoday", s.getActiveVisitorsToday)
					r.Get("/activenonstaff", s.getActiveNonStaff)
					r.Get("/cancelled", s.getCancelledUsers)
					r.Get("/visitors", s.getVisitors)
					r.Get("/contractors", s.getContractors)
					r.Get("/cleaners", s.getCleaners)
					r.Get("/customers", s.getCustomers)
					r.Get("/staff", s.getStaff)
					r.Get("/blankpicture", s.getBlankPicture)
					r.Get("/userpicturebylocalid/{localID:[0-9]+}", s.getUserPictureByLocalID)
					r.With(s.validateUserID).Route("/{userID:[0-9]+}", func(r chi.Router) {
						r.Get("/", s.getUser)
						r.Get("/picture", s.getUserPicture)
						r.Post("/resetantipassback", s.resetAntiPassback)
						r.Post("/activate", s.activateUser)
						r.Post("/deactivate", s.deactivateUser)
						r.Post("/activateAndUpdate", s.activateAndUpdate)
						r.Post("/deactivateAndUpdate", s.deactivateAndUpdate)
						r.Post("/extendexpiry", s.extendExpiry)
						r.Post("/setaccesslevel", s.setAccessLevel)
						r.Post("/addaccesslevel", s.addAccessLevel)
						r.Post("/removeaccesslevel", s.removeAccessLevel)
						r.Post("/changedepartment", s.changeDepartment)
					})
				})
			})
		})
	})
	return r
}

func (s *Server) validateSiteID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siteID, err := strconv.Atoi(chi.URLParam(r, "siteID"))
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "siteID must be numeric"})
			return
		}
		site := s.Sites.GetSite(siteID)
		if site == nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "siteID not found"})
			return
		}
		next.ServeHTTP(w, withValue(r, siteContextKey, site))
	})
}

func (s *Server) validateDoorID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doorID, err := strconv.ParseUint(chi.URLParam(r, "doorID"), 10, 64)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "doorID must be numeric"})
			return
		}
		if s.site(r).GetDoor(doorID) == nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "doorID not found"})
			return
		}
		next.ServeHTTP(w, withValue(r, doorIDContextKey, doorID))
	})
}

func (s *Server) validateUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, err := strconv.Atoi(chi.URLParam(r, "userID"))
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "userID must be numeric"})
			return
		}
		if s.site(r).GetUser(uid) == nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "userID not found"})
			return
		}
		next.ServeHTTP(w, withValue(r, userIDContextKey, uid))
	})
}

func (s *Server) validateOpenableDoor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doorName, err := url.PathUnescape(chi.URLParam(r, "doorName"))
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "invalid door name encoding"})
			return
		}
		if doorName == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "doorName is required"})
			return
		}
		if _, exists := s.site(r).GetOpenableDoors()[doorName]; !exists {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "openable door not found"})
			return
		}
		next.ServeHTTP(w, withValue(r, doorNameContextKey, doorName))
	})
}

func (s *Server) Index(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Net2 Proxy API Index"})
}

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetUsers())
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetUser(s.userID(r)))
}

func (s *Server) getUserPicture(w http.ResponseWriter, r *http.Request) {
	picture, err := s.site(r).GetUserPicture(s.userID(r))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, MessageResponse{Error: "Error getting picture"})
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	render.Status(r, http.StatusOK)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(picture)
}

func (s *Server) getUserPictureByLocalID(w http.ResponseWriter, r *http.Request) {
	localID, _ := strconv.Atoi(chi.URLParam(r, "localID"))
	picture, err := s.site(r).GetUserPictureByLocalID(localID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, MessageResponse{Error: "Error getting picture"})
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	render.Status(r, http.StatusOK)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(picture)
}

func (s *Server) getBlankPicture(w http.ResponseWriter, r *http.Request) {
	picture, err := s.site(r).GetBlankPicture()
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, MessageResponse{Error: "Error getting picture"})
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	render.Status(r, http.StatusOK)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(picture)
}

func (s *Server) getSites(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSites())
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r))
}

func (s *Server) getActiveStaffToday(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveStaffToday())
}

func (s *Server) getActiveStaff(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveStaff())
}

func (s *Server) getActiveVisitors(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveVisitors())
}

func (s *Server) getActiveVisitorsToday(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveVisitorsToday())
}

func (s *Server) getActiveNonStaff(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveNonStaff())
}

func (s *Server) getActiveUsersToday(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveUsersToday())
}

func (s *Server) getActiveUsers(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetActiveUsers())
}

func (s *Server) getCancelledUsers(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetCancelledUsers())
}

func (s *Server) getVisitors(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetVisitors())
}

func (s *Server) getContractors(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetContractors())
}

func (s *Server) getCleaners(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetCleaners())
}

func (s *Server) getCustomers(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetCustomers())
}

func (s *Server) getStaff(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetStaff())
}

func (s *Server) getDoors(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetDoors())
}

func (s *Server) getMonitoredDoors(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetMonitoredDoors())
}

func (s *Server) getOpenableDoors(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetOpenableDoors())
}

func (s *Server) getDoor(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetDoor(s.doorID(r)))
}

func (s *Server) openDoor(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).OpenDoor(s.doorID(r))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error opening door"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Door opened"})
}

func (s *Server) relay1(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).OpenDoorWithRelay(s.doorID(r), false)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error opening door"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Door opened"})
}

func (s *Server) relay2(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).OpenDoorWithRelay(s.doorID(r), true)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error opening door"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Door opened"})
}

func (s *Server) closeDoor(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).CloseDoor(s.doorID(r))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error closing door"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Door closed"})
}

func (s *Server) resetAntiPassback(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).ResetAntiPassback(s.userID(r))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error resetting anti passback"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Anti passback reset"})
}

func (s *Server) getAccessLevels(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetAccessLevels())
}

func (s *Server) getDepartments(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetDepartments())
}

func (s *Server) activateUser(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).ActivateUser(s.userID(r))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error activating user"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User activated"})
}

func (s *Server) deactivateUser(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).DeactivateUser(s.userID(r))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error deactivating user"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User deactivated"})
}

func (s *Server) activateAndUpdate(w http.ResponseWriter, r *http.Request) {
	data := &UpdateUserData{}
	err := render.Bind(r, data)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: err.Error()})
		return
	}
	err = s.site(r).UpdateUserNameAndExpiryAndAccessLevel(
		s.userID(r),
		data.FirstName,
		data.LastName,
		net2.GetTomorrow(),
		data.AccessLevel,
	)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error activating user"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User activated"})
}

func (s *Server) deactivateAndUpdate(w http.ResponseWriter, r *http.Request) {
	data := &UpdateUserData{}
	err := render.Bind(r, data)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: err.Error()})
		return
	}
	err = s.site(r).UpdateUserNameAndExpiryAndAccessLevel(
		s.userID(r),
		data.FirstName,
		data.LastName,
		net2.GetYesterday(),
		data.AccessLevel,
	)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error deactivating user"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User deactivated"})
}

func (s *Server) extendExpiry(w http.ResponseWriter, r *http.Request) {
	err := s.site(r).UpdateUserInfo(s.userID(r), map[string]any{"ExpiryDate": net2.GetTomorrow()})
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error extending user expiry"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User expiry extended"})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	go func() {
		s.Sites.UpdateAll()
	}()
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Update triggered"})
}

func (s *Server) updateNow(w http.ResponseWriter, r *http.Request) {
	s.Sites.UpdateAll()
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Update complete"})
}

func (s *Server) sequenceDoors(w http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	defer func() {
		_ = r.Body.Close()
	}()
	if err != nil {
		log.Error().Err(err).Msg("Unable to read body for door sequence")
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error sequencing doors"})
		return
	}
	log.Debug().Str("Web Door Sequence", string(bytes)).Msg("Raw door data")
	var data []SequenceDoorData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		log.Error().Err(err).Msg("Unable to decode data for door sequence")
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: "Error sequencing doors"})
		return
	}
	log.Debug().Interface("Web Door Sequence", data).Msg("Door data")
	var doors []net2.DoorSequenceItem
	for index := range data {
		if data[index].Time == "" {
			data[index].Time = "0s"
		}
		duration, err := time.ParseDuration(data[index].Time)
		if err != nil {
			log.Error().Err(err).Msg("Unable to decode data duration for door sequence")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "Error sequencing doors"})
			return
		}
		door, err := strconv.ParseUint(data[index].Door, 0, 64)
		if err != nil {
			log.Error().Err(err).Msg("Unable to decode data door for door sequence")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "Error sequencing doors"})
			return
		}
		doors = append(doors, net2.DoorSequenceItem{
			Door: door,
			Time: duration,
		})
	}
	site := s.site(r)
	go func() {
		site.SequenceDoor(doors...)
	}()
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Sequence triggered"})
}

func (s *Server) addAccessLevel(w http.ResponseWriter, r *http.Request) {
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: "level needs to be numeric"})
		return
	}
	err = s.site(r).AddUserAccessLevel(s.userID(r), level)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error adding access level"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User access level added"})
}

func (s *Server) removeAccessLevel(w http.ResponseWriter, r *http.Request) {
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: "level needs to be numeric"})
		return
	}
	err = s.site(r).RemoveUserAccessLevel(s.userID(r), level)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error removing access level"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User access level removed"})
}

func (s *Server) setAccessLevel(w http.ResponseWriter, r *http.Request) {
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: "level needs to be numeric"})
		return
	}
	err = s.site(r).SetUserAccessLevel(s.userID(r), level)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error setting access level"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User access level set"})
}

func (s *Server) getUpToDate(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).UpToDate())
}

func (s *Server) getUnknownTokens(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.site(r).GetUnknownTokens())
}

func (s *Server) changeDepartment(w http.ResponseWriter, r *http.Request) {
	department, err := strconv.Atoi(r.URL.Query().Get("department"))
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, MessageResponse{Error: "department needs to be numeric"})
		return
	}
	err = s.site(r).ChangeUserDepartment(s.userID(r), department)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error changing user department"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User department set"})
}

func (s *Server) openOpenableDoor(w http.ResponseWriter, r *http.Request) {
	site := s.site(r)
	sequence := site.GetOpenableDoors()[s.doorName(r)]

	var doors []net2.DoorSequenceItem
	for _, item := range sequence {
		doors = append(doors, net2.DoorSequenceItem{
			Door: uint64(item.ID),
			Time: time.Duration(item.Duration),
		})
	}

	go func() {
		site.SequenceDoor(doors...)
	}()

	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Openable door sequence triggered"})
}

func (s *Server) validateDepartmentName(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		departmentName, err := url.PathUnescape(chi.URLParam(r, "departmentName"))
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "invalid department name encoding"})
			return
		}
		if departmentName == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "departmentName is required"})
			return
		}
		found := false
		for _, dept := range s.site(r).GetDepartments() {
			if dept.Name == departmentName {
				found = true
				break
			}
		}
		if !found {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "department not found"})
			return
		}
		next.ServeHTTP(w, withValue(r, departmentNameContextKey, departmentName))
	})
}

func (s *Server) activateDepartmentUsers(w http.ResponseWriter, r *http.Request) {
	departmentName := s.departmentName(r)
	site := s.site(r)
	users := site.GetUsers()

	var activatedCount int
	var failedCount int

	for _, user := range users {
		inDepartment := false
		for _, dept := range user.Departments {
			if dept.Name == departmentName {
				inDepartment = true
				break
			}
		}

		if inDepartment {
			err := site.ActivateUser(user.ID)
			if err != nil {
				log.Error().Err(err).Int("userID", user.ID).Str("department", departmentName).Msg("Failed to activate user")
				failedCount++
			} else {
				activatedCount++
			}
		}
	}

	if failedCount > 0 {
		render.Status(r, http.StatusPartialContent)
		render.JSON(w, r, MessageResponse{
			Message: fmt.Sprintf("Activated %d users in department '%s', %d failed", activatedCount, departmentName, failedCount),
		})
	} else {
		render.Status(r, http.StatusOK)
		render.JSON(w, r, MessageResponse{
			Message: fmt.Sprintf("Successfully activated %d users in department '%s'", activatedCount, departmentName),
		})
	}
}
