package dto

import (
	"time"

	"github.com/wishwall/wishwall/internal/model"
)

// ApplyExtensionRequest 圆梦人提交延期协商入参（新的截止日 + 原因）。
type ApplyExtensionRequest struct {
	NewDeadline time.Time `json:"new_deadline" binding:"required"`
	Reason      string    `json:"reason" binding:"required,min=2,max=500"`
}

// ReviewExtensionRequest 发布者审核延期协商入参。
type ReviewExtensionRequest struct {
	Approved bool   `json:"approved"`
	Comment  string `json:"comment" binding:"omitempty,max=500"`
}

// ExtensionResponse 延期协商返回结构（详情页按双方身份展示状态与操作入口）。
type ExtensionResponse struct {
	ID               uint64  `json:"id"`
	WishID           uint64  `json:"wish_id"`
	ClaimID          uint64  `json:"claim_id"`
	UserID           uint64  `json:"user_id"`
	FulfillerName    string  `json:"fulfiller_name"`
	OriginalDeadline *string `json:"original_deadline"`
	NewDeadline      string  `json:"new_deadline"`
	Reason           string  `json:"reason"`
	Status           string  `json:"status"`
	ReviewerID       uint64  `json:"reviewer_id"`
	ReviewComment    string  `json:"review_comment"`
	ReviewedAt       *string `json:"reviewed_at"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// ToExtensionResponse 从模型构造返回结构。
func ToExtensionResponse(e *model.DeadlineExtension, fulfillerName string) ExtensionResponse {
	resp := ExtensionResponse{
		ID:            e.ID,
		WishID:        e.WishID,
		ClaimID:       e.ClaimID,
		UserID:        e.UserID,
		FulfillerName: fulfillerName,
		NewDeadline:   e.NewDeadline.Format("2006-01-02"),
		Reason:        e.Reason,
		Status:        e.Status,
		ReviewerID:    e.ReviewerID,
		ReviewComment: e.ReviewComment,
		CreatedAt:     e.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:     e.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if e.OriginalDeadline != nil {
		s := e.OriginalDeadline.Format("2006-01-02")
		resp.OriginalDeadline = &s
	}
	if e.ReviewedAt != nil {
		s := e.ReviewedAt.Format("2006-01-02 15:04:05")
		resp.ReviewedAt = &s
	}
	return resp
}
