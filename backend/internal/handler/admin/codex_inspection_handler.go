package admin

import (
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CodexInspectionHandler struct {
	service *service.CodexInspectionService
}

func NewCodexInspectionHandler(svc *service.CodexInspectionService) *CodexInspectionHandler {
	return &CodexInspectionHandler{service: svc}
}

func (h *CodexInspectionHandler) Overview(c *gin.Context) {
	overview, err := h.service.Overview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *CodexInspectionHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *CodexInspectionHandler) UpdateSettings(c *gin.Context) {
	var req service.CodexInspectionSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusUnprocessableEntity, "INVALID_CODEX_INSPECTION_SETTINGS", err.Error()))
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *CodexInspectionHandler) CreateRun(c *gin.Context) {
	var req service.CodexInspectionRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusUnprocessableEntity, "INVALID_CODEX_INSPECTION_RUN_REQUEST", err.Error()))
		return
	}
	run, err := h.service.StartRun(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, run)
}

func (h *CodexInspectionHandler) ListRuns(c *gin.Context) {
	limit, offset := parseLimitOffset(c, 50)
	page, err := h.service.ListRuns(c.Request.Context(), service.CodexInspectionListRunsParams{
		Status:      strings.TrimSpace(c.Query("status")),
		TriggerType: strings.TrimSpace(c.Query("trigger_type")),
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, page)
}

func (h *CodexInspectionHandler) GetRun(c *gin.Context) {
	id, ok := parseCodexInspectionID(c, "id")
	if !ok {
		return
	}
	detail, err := h.service.GetRun(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *CodexInspectionHandler) ListRunResults(c *gin.Context) {
	id, ok := parseCodexInspectionID(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	results, err := h.service.ListResults(c.Request.Context(), service.CodexInspectionListResultsParams{
		RunID:            id,
		Page:             page,
		PageSize:         pageSize,
		Action:           strings.TrimSpace(c.Query("action")),
		ProbeStatus:      strings.TrimSpace(c.Query("probe_status")),
		AccountStatus:    strings.TrimSpace(c.Query("account_status")),
		QuotaWindow:      strings.TrimSpace(c.Query("quota_window")),
		GroupIDs:         parseCodexInspectionIDList(append(c.QueryArray("group_ids"), c.QueryArray("group_ids[]")...)),
		OnlyStaleMinutes: parseCodexInspectionPositiveInt(c.Query("only_stale_minutes")),
		Search:           strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, results)
}

func (h *CodexInspectionHandler) CancelRun(c *gin.Context) {
	id, ok := parseCodexInspectionID(c, "id")
	if !ok {
		return
	}
	run, err := h.service.CancelRun(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, run)
}

func (h *CodexInspectionHandler) ApplyRunActions(c *gin.Context) {
	id, ok := parseCodexInspectionID(c, "id")
	if !ok {
		return
	}
	var req service.CodexInspectionActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusUnprocessableEntity, "INVALID_CODEX_INSPECTION_ACTION_REQUEST", err.Error()))
		return
	}
	outcomes, err := h.service.ExecuteActions(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": outcomes})
}

func (h *CodexInspectionHandler) ProbeAccount(c *gin.Context) {
	accountID, ok := parseCodexInspectionID(c, "account_id")
	if !ok {
		return
	}
	detail, err := h.service.ProbeAccount(c.Request.Context(), accountID)
	if err != nil {
		if infraerrors.Code(err) == http.StatusBadGateway && detail != nil {
			c.JSON(http.StatusBadGateway, response.Response{
				Code:    http.StatusBadGateway,
				Message: infraerrors.Message(err),
				Reason:  infraerrors.Reason(err),
				Data:    detail,
			})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

func (h *CodexInspectionHandler) LatestAccounts(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	results, err := h.service.ListLatestAccountResults(c.Request.Context(), service.CodexInspectionLatestResultsParams{
		Page:             page,
		PageSize:         pageSize,
		Action:           strings.TrimSpace(c.Query("action")),
		ProbeStatus:      strings.TrimSpace(c.Query("probe_status")),
		AccountStatus:    strings.TrimSpace(c.Query("account_status")),
		QuotaWindow:      strings.TrimSpace(c.Query("quota_window")),
		GroupIDs:         parseCodexInspectionIDList(append(c.QueryArray("group_ids"), c.QueryArray("group_ids[]")...)),
		OnlyStaleMinutes: parseCodexInspectionPositiveInt(c.Query("only_stale_minutes")),
		Search:           strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, results)
}

func (h *CodexInspectionHandler) ListLogs(c *gin.Context) {
	limit, offset := parseLimitOffset(c, 50)
	runID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("run_id")), 10, 64)
	accountID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("account_id")), 10, 64)
	logs, err := h.service.ListLogs(c.Request.Context(), service.CodexInspectionListLogsParams{
		RunID:     runID,
		AccountID: accountID,
		Level:     strings.TrimSpace(c.Query("level")),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, logs)
}

func parseCodexInspectionID(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_CODEX_INSPECTION_ID", "invalid id"))
		return 0, false
	}
	return id, true
}

func parseLimitOffset(c *gin.Context, defaultLimit int) (int, int) {
	limit := defaultLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			offset = v
		}
	}
	return limit, offset
}

func parseCodexInspectionIDList(values []string) []int64 {
	seen := make(map[int64]struct{})
	out := make([]int64, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func parseCodexInspectionPositiveInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
