package dto

import (
	"time"

	"github.com/wishwall/wishwall/internal/model"
)

// RequestExtensionRequest 圆梦人提交延期申请入参（新截止日 + 原因）。
type RequestExtensionRequest struct {
	NewDeadline time.Time `json:"new_deadline" binding:"required"`
	Reason      string    `json:"reason" binding:"required,min=2,max=500"`
}

// WishExtensionResponse 延期申请返回结构。
type WishExtensionResponse struct {
	ID          uint64 `json:"id"`
	ClaimID     uint64 `json:"claim_id"`
	WishID      uint64 `json:"wish_id"`
	UserID      uint64 `json:"user_id"`
	NewDeadline string `json:"new_deadline"`
	Reason      string `json:"reason"`
	Status      string `json:"status"`
	StatusText  string `json:"status_text"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ToWishExtensionResponse 从模型构造返回结构。
func ToWishExtensionResponse(e *model.WishExtension, statusText string) WishExtensionResponse {
	return WishExtensionResponse{
		ID:          e.ID,
		ClaimID:     e.ClaimID,
		WishID:      e.WishID,
		UserID:      e.UserID,
		NewDeadline: e.NewDeadline.Format("2006-01-02"),
		Reason:      e.Reason,
		Status:      e.Status,
		StatusText:  statusText,
		CreatedAt:   e.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   e.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
