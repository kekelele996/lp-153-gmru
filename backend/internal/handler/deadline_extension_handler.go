package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/middleware"
	"github.com/wishwall/wishwall/internal/service"
)

// DeadlineExtensionHandler 截止日延期协商处理器。
type DeadlineExtensionHandler struct {
	extension service.DeadlineExtensionService
}

// NewDeadlineExtensionHandler 构造延期协商处理器。
func NewDeadlineExtensionHandler(extension service.DeadlineExtensionService) *DeadlineExtensionHandler {
	return &DeadlineExtensionHandler{extension: extension}
}

// Apply POST /api/v1/wishes/:id/extension
func (h *DeadlineExtensionHandler) Apply(c *gin.Context) {
	wishID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "心愿 id 参数非法")
		return
	}
	var req dto.ApplyExtensionRequest
	if !bindJSON(c, &req) {
		return
	}
	userID := middleware.CurrentUserID(c)
	extension, err := h.extension.Apply(c.Request.Context(), userID, wishID, req, c.ClientIP(), middleware.GetRequestID(c))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": constants.MsgExtensionApplied, "data": dto.ToExtensionResponse(extension, "")})
}

// Review POST /api/v1/wishes/:id/extension/review
func (h *DeadlineExtensionHandler) Review(c *gin.Context) {
	wishID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "心愿 id 参数非法")
		return
	}
	var req dto.ReviewExtensionRequest
	if !bindJSON(c, &req) {
		return
	}
	userID := middleware.CurrentUserID(c)
	extension, err := h.extension.Review(c.Request.Context(), userID, wishID, req, c.ClientIP(), middleware.GetRequestID(c))
	if err != nil {
		handleError(c, err)
		return
	}
	message := constants.MsgExtensionRejected
	if req.Approved {
		message = constants.MsgExtensionApproved
	}
	c.JSON(200, gin.H{"code": 0, "message": message, "data": dto.ToExtensionResponse(extension, "")})
}

// GetByWish GET /api/v1/wishes/:id/extension
func (h *DeadlineExtensionHandler) GetByWish(c *gin.Context) {
	wishID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "心愿 id 参数非法")
		return
	}
	userID := middleware.CurrentUserID(c)
	resp, err := h.extension.GetByWishID(userID, wishID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": resp})
}
