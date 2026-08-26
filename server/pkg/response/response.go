package response

import (
	"errors"
	"net/http"

	"backupx/server/internal/apperror"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const loggerContextKey = "backupx.response.logger"

type Envelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: "OK", Message: "success", Data: data})
}

// SetLogger attaches the application logger to a request so response helpers
// can report failures without relying on a process-global logger.
func SetLogger(c *gin.Context, logger *zap.Logger) {
	if logger != nil {
		c.Set(loggerContextKey, logger)
	}
}

func Error(c *gin.Context, err error) {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		logError(c, appErr.Status, appErr.Code, err)
		c.JSON(appErr.Status, Envelope{Code: appErr.Code, Message: appErr.Message})
		return
	}
	logError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err)
	c.JSON(http.StatusInternalServerError, Envelope{Code: "INTERNAL_ERROR", Message: "服务器内部错误"})
}

func logError(c *gin.Context, status int, code string, err error) {
	value, exists := c.Get(loggerContextKey)
	if !exists {
		return
	}
	logger, ok := value.(*zap.Logger)
	if !ok || logger == nil {
		return
	}
	method := ""
	path := ""
	if c.Request != nil {
		method = c.Request.Method
		path = c.Request.URL.Path
	}
	fields := []zap.Field{
		zap.Int("status", status),
		zap.String("code", code),
		zap.String("method", method),
		zap.String("path", path),
		zap.Error(err),
	}
	if status >= http.StatusInternalServerError {
		logger.Error("http request failed", fields...)
		return
	}
	logger.Warn("http request rejected", fields...)
}
