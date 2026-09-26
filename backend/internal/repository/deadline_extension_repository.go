package repository

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wishwall/wishwall/internal/model"
)

// DeadlineExtensionRepository 截止日延期协商仓储接口。
type DeadlineExtensionRepository interface {
	Create(extension *model.DeadlineExtension) error
	CreateWithTx(tx *gorm.DB, extension *model.DeadlineExtension) error
	FindByID(id uint64) (*model.DeadlineExtension, error)
	FindByWishID(wishID uint64) (*model.DeadlineExtension, error)
	FindByWishIDForUpdate(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error)
	Update(extension *model.DeadlineExtension) error
	UpdateWithTx(tx *gorm.DB, extension *model.DeadlineExtension) error
}

type deadlineExtensionRepository struct {
	db *gorm.DB
}

// NewDeadlineExtensionRepository 构造延期协商仓储。
func NewDeadlineExtensionRepository(db *gorm.DB) DeadlineExtensionRepository {
	return &deadlineExtensionRepository{db: db}
}

func (r *deadlineExtensionRepository) Create(extension *model.DeadlineExtension) error {
	if err := r.db.Create(extension).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create extension on wish %d: %w", extension.WishID, ErrConflict)
		}
		return fmt.Errorf("create extension on wish %d: %w", extension.WishID, err)
	}
	return nil
}

func (r *deadlineExtensionRepository) CreateWithTx(tx *gorm.DB, extension *model.DeadlineExtension) error {
	if err := tx.Create(extension).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create extension on wish %d with tx: %w", extension.WishID, ErrConflict)
		}
		return fmt.Errorf("create extension on wish %d with tx: %w", extension.WishID, err)
	}
	return nil
}

func (r *deadlineExtensionRepository) FindByID(id uint64) (*model.DeadlineExtension, error) {
	var extension model.DeadlineExtension
	if err := r.db.First(&extension, id).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by id %d: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by id %d: %w", id, err)
	}
	return &extension, nil
}

func (r *deadlineExtensionRepository) FindByWishID(wishID uint64) (*model.DeadlineExtension, error) {
	var extension model.DeadlineExtension
	if err := r.db.Where("wish_id = ?", wishID).First(&extension).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by wish %d: %w", wishID, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by wish %d: %w", wishID, err)
	}
	return &extension, nil
}

// FindByWishIDForUpdate 行级锁读取（FOR UPDATE），并发提交/审核同一条申请时串行化。
func (r *deadlineExtensionRepository) FindByWishIDForUpdate(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) {
	var extension model.DeadlineExtension
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("wish_id = ?", wishID).First(&extension).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by wish %d for update: %w", wishID, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by wish %d for update: %w", wishID, err)
	}
	return &extension, nil
}

func (r *deadlineExtensionRepository) Update(extension *model.DeadlineExtension) error {
	if err := r.db.Save(extension).Error; err != nil {
		return fmt.Errorf("update extension %d: %w", extension.ID, err)
	}
	return nil
}

func (r *deadlineExtensionRepository) UpdateWithTx(tx *gorm.DB, extension *model.DeadlineExtension) error {
	if err := tx.Save(extension).Error; err != nil {
		return fmt.Errorf("update extension %d with tx: %w", extension.ID, err)
	}
	return nil
}
