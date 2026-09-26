package router

import (
	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/handler"
)

// RegisterDeadlineExtensionRoutes 注册截止日延期协商路由。
func RegisterDeadlineExtensionRoutes(rg *gin.RouterGroup, h *handler.DeadlineExtensionHandler, auth gin.HandlerFunc) {
	rg.POST("/wishes/:id/extension", auth, h.Apply)
	rg.POST("/wishes/:id/extension/review", auth, h.Review)
	rg.GET("/wishes/:id/extension", auth, h.GetByWish)
}
