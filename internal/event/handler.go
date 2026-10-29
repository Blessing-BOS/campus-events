package event

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
	log     *slog.Logger
}

func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

type ErrorResponse struct {
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WriteError(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message}, RequestID: c.GetString("request_id")})
}

func (h *Handler) List(c *gin.Context) {
	limit := 20
	query := c.Request.URL.Query()
	if values, exists := query["limit"]; exists {
		if len(values) != 1 {
			WriteError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "limit must appear once")
			return
		}
		value, err := strconv.Atoi(values[0])
		if err != nil || value < 1 || value > 100 {
			WriteError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "limit must be between 1 and 100")
			return
		}
		limit = value
	}
	var beforeID uint64
	if values, exists := query["before_id"]; exists {
		if len(values) != 1 {
			WriteError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "before_id must appear once")
			return
		}
		value, err := strconv.ParseUint(values[0], 10, 64)
		if err != nil || value == 0 {
			WriteError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "before_id must be a positive integer")
			return
		}
		beforeID = value
	}
	result, err := h.service.List(c.Request.Context(), beforeID, limit)
	if err != nil {
		h.log.Error("list events failed", "error", err)
		WriteError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "unable to load events")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		WriteError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "id must be a positive integer")
		return
	}
	result, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		WriteError(c, http.StatusNotFound, "EVENT_NOT_FOUND", "event not found")
		return
	}
	if err != nil {
		h.log.Error("get event failed", "event_id", id, "error", err)
		WriteError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "unable to load event")
		return
	}
	c.JSON(http.StatusOK, result)
}
