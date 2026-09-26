package model

import "time"

// DeadlineExtension 截止日延期协商实体。同一认领（同一心愿）仅保留一份申请（wish_id 唯一索引兜底），
// 圆梦人重复申请为 upsert（覆盖新的截止日与原因，状态回到 pending）。
// 状态机：pending -> approved / rejected；rejected 后允许再次申请（重新回到 pending）。
type DeadlineExtension struct {
	ID               uint64     `gorm:"primaryKey" json:"id"`
	WishID           uint64     `gorm:"uniqueIndex;not null" json:"wish_id"`
	ClaimID          uint64     `gorm:"index;not null" json:"claim_id"`
	UserID           uint64     `gorm:"index;not null" json:"user_id"`
	OriginalDeadline *time.Time `json:"original_deadline"`
	NewDeadline      time.Time  `gorm:"not null" json:"new_deadline"`
	Reason           string     `gorm:"type:text;not null" json:"reason"`
	Status           string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	ReviewerID       uint64     `gorm:"not null;default:0" json:"reviewer_id"`
	ReviewComment    string     `gorm:"type:text" json:"review_comment"`
	ReviewedAt       *time.Time `json:"reviewed_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
