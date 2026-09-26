package model

import "time"

// WishExtension 延期协商申请（圆梦人发起、发布者裁决）。同一认领仅保留一条申请（claim_id 唯一索引兜底并发）。
type WishExtension struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	ClaimID     uint64    `gorm:"uniqueIndex;not null" json:"claim_id"`
	WishID      uint64    `gorm:"index;not null" json:"wish_id"`
	UserID      uint64    `gorm:"index;not null" json:"user_id"`
	NewDeadline time.Time `gorm:"not null" json:"new_deadline"`
	Reason      string    `gorm:"type:text;not null" json:"reason"`
	Status      string    `gorm:"size:20;index;not null;default:pending" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
