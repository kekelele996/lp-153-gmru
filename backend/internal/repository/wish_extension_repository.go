package repository

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/model"
)

// WishExtensionRepository 延期协商申请仓储接口。
type WishExtensionRepository interface {
	CreateWithTx(tx *gorm.DB, ext *model.WishExtension) error
	FindByClaimID(claimID uint64) (*model.WishExtension, error)
	FindByWishID(wishID uint64) (*model.WishExtension, error)
	UpdateWithTx(tx *gorm.DB, ext *model.WishExtension) error
}

type wishExtensionRepository struct {
	db *gorm.DB
}

// NewWishExtensionRepository 构造延期申请仓储。
func NewWishExtensionRepository(db *gorm.DB) WishExtensionRepository {
	return &wishExtensionRepository{db: db}
}

func (r *wishExtensionRepository) CreateWithTx(tx *gorm.DB, ext *model.WishExtension) error {
	if err := tx.Create(ext).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create extension on claim %d: %w", ext.ClaimID, ErrConflict)
		}
		return fmt.Errorf("create extension on claim %d: %w", ext.ClaimID, err)
	}
	return nil
}

func (r *wishExtensionRepository) FindByClaimID(claimID uint64) (*model.WishExtension, error) {
	var ext model.WishExtension
	if err := r.db.Where("claim_id = ?", claimID).First(&ext).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by claim %d: %w", claimID, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by claim %d: %w", claimID, err)
	}
	return &ext, nil
}

func (r *wishExtensionRepository) FindByWishID(wishID uint64) (*model.WishExtension, error) {
	var ext model.WishExtension
	if err := r.db.Where("wish_id = ?", wishID).First(&ext).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by wish %d: %w", wishID, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by wish %d: %w", wishID, err)
	}
	return &ext, nil
}

func (r *wishExtensionRepository) UpdateWithTx(tx *gorm.DB, ext *model.WishExtension) error {
	if err := tx.Save(ext).Error; err != nil {
		return fmt.Errorf("update extension %d with tx: %w", ext.ID, err)
	}
	return nil
}
