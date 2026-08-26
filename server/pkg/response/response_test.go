package response

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorLogsInternalFailureAndHidesDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	core, observed := observer.New(zap.WarnLevel)
	SetLogger(ctx, zap.New(core))

	Error(ctx, errors.New("database password should stay private"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "database password") {
		t.Fatalf("internal detail leaked in response: %s", recorder.Body.String())
	}
	entries := observed.All()
	if len(entries) != 1 || entries[0].Level != zap.ErrorLevel {
		t.Fatalf("observed logs = %#v, want one error", entries)
	}
	context := entries[0].ContextMap()
	if context["code"] != "INTERNAL_ERROR" || context["path"] != "/api/test" {
		t.Fatalf("unexpected log context: %#v", context)
	}
}
