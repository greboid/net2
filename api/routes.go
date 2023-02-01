package api

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/greboid/net2/net2"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
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
		return errors.New("missing required field FirstName")
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

func (s *Server) GetRoutes() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
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
				})
				r.Route("/doors", func(r chi.Router) {
					r.Get("/", s.getDoors)
					r.Get("/monitored", s.getMonitoredDoors)
					r.Get("/openable", s.getOpenableDoors)
					r.Post("/sequence", s.sequenceDoors)
					r.With(s.validateDoorID).Route("/{doorID:[0-9]+}", func(r chi.Router) {
						r.Get("/", s.getDoor)
						r.Post("/open", s.openDoor)
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
		if s.Sites.GetSite(siteID) == nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "siteID not found"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validateDoorID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
		doorID, err := strconv.ParseUint(chi.URLParam(r, "doorID"), 0, 64)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "doorID must be numeric"})
			return
		}
		if s.Sites.GetSite(siteID).GetDoor(doorID) == nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "doorID not found"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validateUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
		userID, err := strconv.Atoi(chi.URLParam(r, "userID"))
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, MessageResponse{Error: "userID must be numeric"})
			return
		}
		if s.Sites.GetSite(siteID).GetUser(userID) == nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, MessageResponse{Error: "userID not found"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Index(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Net2 Proxy API Index"})
}

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetUsers())
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetUser(userID))
}

func (s *Server) getUserPicture(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	picture, err := s.Sites.GetSite(siteID).GetUserPicture(userID)
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

func (s *Server) getSites(w http.ResponseWriter, r *http.Request) {
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSites())
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID))
}

func (s *Server) getActiveStaffToday(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveStaffToday())
}

func (s *Server) getActiveStaff(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveStaff())
}

func (s *Server) getActiveVisitors(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveVisitors())
}

func (s *Server) getActiveVisitorsToday(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveVisitorsToday())
}

func (s *Server) getActiveNonStaff(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveNonStaff())
}

func (s *Server) getActiveUsersToday(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveUsersToday())
}

func (s *Server) getActiveUsers(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetActiveUsers())
}

func (s *Server) getCancelledUsers(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetCancelledUsers())
}

func (s *Server) getVisitors(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetVisitors())
}

func (s *Server) getContractors(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetContractors())
}

func (s *Server) getCleaners(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetCleaners())
}

func (s *Server) getCustomers(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetCustomers())
}

func (s *Server) getStaff(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetStaff())
}

func (s *Server) getDoors(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetDoors())
}

func (s *Server) getMonitoredDoors(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetMonitoredDoors())
}

func (s *Server) getOpenableDoors(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetOpenableDoors())
}

func (s *Server) getDoor(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	doorID, _ := strconv.Atoi(chi.URLParam(r, "doorID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetDoor(uint64(doorID)))
}

func (s *Server) openDoor(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	doorID, _ := strconv.Atoi(chi.URLParam(r, "doorID"))
	err := s.Sites.GetSite(siteID).OpenDoor(uint64(doorID))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error opening door"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Door opened"})
}

func (s *Server) closeDoor(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	doorID, _ := strconv.Atoi(chi.URLParam(r, "doorID"))
	err := s.Sites.GetSite(siteID).CloseDoor(uint64(doorID))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error closing door"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Door closed"})
}

func (s *Server) resetAntiPassback(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	err := s.Sites.GetSite(siteID).ResetAntiPassback(userID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error resetting anti passback"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Anti passback reset"})
}

func (s *Server) getAccessLevels(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetAccessLevels())
}

func (s *Server) getDepartments(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).GetDepartments())
}

func (s *Server) activateUser(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	err := s.Sites.GetSite(siteID).ActivateUser(userID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error activating user"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User activated"})
}

func (s *Server) deactivateUser(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	err := s.Sites.GetSite(siteID).DeactivateUser(userID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error deactivating user"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User deactivated"})
}

func (s *Server) activateAndUpdate(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	data := &UpdateUserData{}
	err := render.Bind(r, data)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error activating user"})
		return
	}
	err = s.Sites.GetSite(siteID).UpdateUserNameAndExpiryAndAccessLevel(
		userID,
		data.FirstName,
		data.LastName,
		GetTomorrow(),
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
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	data := &UpdateUserData{}
	err := render.Bind(r, data)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "Error activating user"})
		return
	}
	err = s.Sites.GetSite(siteID).UpdateUserNameAndExpiryAndAccessLevel(
		userID,
		data.FirstName,
		data.LastName,
		GetYesterday(),
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
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	err := s.Sites.GetSite(siteID).UpdateUserInfo(userID, map[string]interface{}{"ExpiryDate": GetTomorrow()})
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
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
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
		render.Status(r, http.StatusInternalServerError)
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
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, MessageResponse{Error: "Error sequencing doors"})
			return
		}
		door, err := strconv.ParseUint(data[index].Door, 0, 64)
		if err != nil {
			log.Error().Err(err).Msg("Unable to decode data door for door sequence")
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, MessageResponse{Error: "Error sequencing doors"})
			return
		}
		doors = append(doors, net2.DoorSequenceItem{
			Door: door,
			Time: duration,
		})
	}
	go func() {
		s.Sites.GetSite(siteID).SequenceDoor(doors...)
	}()
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "Sequence triggered"})
}

func (s *Server) addAccessLevel(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "level needs to be numeric"})
		return
	}
	err = s.Sites.GetSite(siteID).AddUserAccessLevel(userID, level)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error adding access level"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User access level added"})
}

func (s *Server) removeAccessLevel(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "level needs to be numeric"})
		return
	}
	err = s.Sites.GetSite(siteID).RemoveUserAccessLevel(userID, level)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error removing access level"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User access level removed"})
}

func (s *Server) setAccessLevel(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "level needs to be numeric"})
		return
	}
	err = s.Sites.GetSite(siteID).SetUserAccessLevel(userID, level)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error setting access level"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User access level set"})
}

func (s *Server) getUpToDate(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, !s.Sites.GetSite(siteID).LastPolled.Before(time.Now().Add(-1*3*time.Minute)))
}

func (s *Server) getUnknownTokens(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	render.Status(r, http.StatusOK)
	render.JSON(w, r, s.Sites.GetSite(siteID).UnknownTokens)
}

func (s *Server) changeDepartment(w http.ResponseWriter, r *http.Request) {
	siteID, _ := strconv.Atoi(chi.URLParam(r, "siteID"))
	userID, _ := strconv.Atoi(chi.URLParam(r, "userID"))
	department, err := strconv.Atoi(r.URL.Query().Get("department"))
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "department needs to be numeric"})
		return
	}
	err = s.Sites.GetSite(siteID).ChangeUserDepartment(userID, department)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, MessageResponse{Error: "error changing user department"})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(w, r, MessageResponse{Message: "User department set"})
}

func GetTomorrow() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 23, 59, 0, 0, time.Local)
}

func GetYesterday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 0, 0, time.Local)
}
