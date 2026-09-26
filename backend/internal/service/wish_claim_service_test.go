package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/model"
	"github.com/wishwall/wishwall/internal/util"
)

func TestWishClaimService_Claim(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		wishFn      func(tx *gorm.DB, id uint64) (*model.Wish, error)
		createFn    func(tx *gorm.DB, claim *model.WishClaim) error
		wantErrCode int
	}{
		{
			name: "claim success",
			wishFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusPending}, nil
			},
			createFn: func(tx *gorm.DB, claim *model.WishClaim) error {
				claim.ID = 7
				return nil
			},
			wantErrCode: 0,
		},
		{
			name: "wish already claimed",
			wishFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusClaimed}, nil
			},
			createFn: func(tx *gorm.DB, claim *model.WishClaim) error { return nil },
			wantErrCode: constants.CodeWishAlreadyClaimed,
		},
		{
			name: "self claim forbidden",
			wishFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 2, Status: constants.WishStatusPending}, nil
			},
			createFn: func(tx *gorm.DB, claim *model.WishClaim) error { return nil },
			wantErrCode: constants.CodeWishStatusInvalid,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			wishRepo := &mockWishRepo{
				findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
					// 转换为真实签名：mock 忽略 tx
					return tt.wishFn(nil, id)
				},
				updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { return nil },
			}
			claimRepo := &mockClaimRepo{
				createWithTxFn: func(tx *gorm.DB, claim *model.WishClaim) error {
					return tt.createFn(nil, claim)
				},
			}
			badge := &mockBadge{grantFirstClaimFn: func(userID uint64) error { return nil }}
			svc := NewWishClaimService(&mockTx{}, wishRepo, claimRepo, &mockExtRepo{}, &mockUserRepo{}, badge, &mockAudit{}, testLogger())
			claim, err := svc.Claim(context.Background(), 2, 5, "127.0.0.1", "req-5")
			if tt.wantErrCode == 0 {
				if err != nil {
					t.Fatalf("claim should succeed, got %v", err)
				}
				if claim.WishID != 5 {
					t.Fatalf("unexpected wish id %d", claim.WishID)
				}
				return
			}
			var appErr *util.AppError
			if !errors.As(err, &appErr) || appErr.Code != tt.wantErrCode {
				t.Fatalf("expected code %d, got %v", tt.wantErrCode, err)
			}
		})
	}
}

func TestWishClaimService_UpdateProgress_Complete(t *testing.T) {
	t.Parallel()
	wishRepo := &mockWishRepo{
		findByIDFn: func(id uint64) (*model.Wish, error) {
			return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusClaimed}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { return nil },
		countCompletedFn: func(userID uint64) (int64, error) { return 11, nil },
	}
	claimRepo := &mockClaimRepo{
		findByIDFn: func(id uint64) (*model.WishClaim, error) {
			return &model.WishClaim{ID: id, WishID: 5, UserID: 2, Progress: 40, Status: constants.WishStatusInProgress}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, claim *model.WishClaim) error { return nil },
	}
	badge := &mockBadge{grantCompletionFn: func(userID uint64) error { return nil }}
	svc := NewWishClaimService(&mockTx{}, wishRepo, claimRepo, &mockExtRepo{}, &mockUserRepo{}, badge, &mockAudit{}, testLogger())

	claim, err := svc.UpdateProgress(context.Background(), 2, 9, dto.UpdateProgressRequest{Progress: 100, Note: "做到了！", IsMilestone: true}, "127.0.0.1", "req-6")
	if err != nil {
		t.Fatalf("update progress: %v", err)
	}
	if claim.Status != constants.WishStatusCompleted {
		t.Fatalf("expected completed, got %s", claim.Status)
	}
	if claim.MilestoneCount != 1 {
		t.Fatalf("expected milestone count 1, got %d", claim.MilestoneCount)
	}
}

// 延期协商待处理期间禁止标记完成。
func TestWishClaimService_Complete_BlockedDuringExtension(t *testing.T) {
	t.Parallel()
	wishRepo := &mockWishRepo{
		findByIDFn: func(id uint64) (*model.Wish, error) {
			return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusExtensionPending}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { return nil },
	}
	claimRepo := &mockClaimRepo{
		findByIDFn: func(id uint64) (*model.WishClaim, error) {
			return &model.WishClaim{ID: id, WishID: 5, UserID: 2, Progress: 80, Status: constants.WishStatusInProgress}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, claim *model.WishClaim) error { return nil },
	}
	svc := NewWishClaimService(&mockTx{}, wishRepo, claimRepo, &mockExtRepo{}, &mockUserRepo{}, &mockBadge{}, &mockAudit{}, testLogger())

	_, err := svc.Complete(context.Background(), 2, 9, dto.CompleteClaimRequest{Note: "提前交卷"}, "127.0.0.1", "req-7")
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Code != constants.CodeClaimStatusInvalid {
		t.Fatalf("expected CodeClaimStatusInvalid, got %v", err)
	}
}

// 圆梦人提交延期申请：心愿转入延期待处理，同一认领只保留一份申请。
func TestWishClaimService_RequestExtension(t *testing.T) {
	t.Parallel()
	newDeadline := time.Now().Add(7 * 24 * time.Hour)
	var savedWish *model.Wish
	wishRepo := &mockWishRepo{
		findByIDFn: func(id uint64) (*model.Wish, error) {
			return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { savedWish = wish; return nil },
	}
	claimRepo := &mockClaimRepo{
		findByIDFn: func(id uint64) (*model.WishClaim, error) {
			return &model.WishClaim{ID: id, WishID: 5, UserID: 2, Progress: 60, Status: constants.WishStatusInProgress}, nil
		},
	}
	extRepo := &mockExtRepo{
		createWithTxFn: func(tx *gorm.DB, ext *model.WishExtension) error { ext.ID = 3; return nil },
	}
	svc := NewWishClaimService(&mockTx{}, wishRepo, claimRepo, extRepo, &mockUserRepo{}, &mockBadge{}, &mockAudit{}, testLogger())

	ext, err := svc.RequestExtension(context.Background(), 2, 9, dto.RequestExtensionRequest{NewDeadline: newDeadline, Reason: "材料还在路上"}, "127.0.0.1", "req-8")
	if err != nil {
		t.Fatalf("request extension: %v", err)
	}
	if ext.Status != constants.ExtensionStatusPending {
		t.Fatalf("expected pending extension, got %s", ext.Status)
	}
	if savedWish == nil || savedWish.Status != constants.WishStatusExtensionPending {
		t.Fatalf("expected wish extension_pending, got %+v", savedWish)
	}

	// 非圆梦人提交被拒绝。
	_, err = svc.RequestExtension(context.Background(), 99, 9, dto.RequestExtensionRequest{NewDeadline: newDeadline, Reason: "别人想延期"}, "127.0.0.1", "req-9")
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Code != constants.CodeClaimNotOwner {
		t.Fatalf("expected CodeClaimNotOwner, got %v", err)
	}

	// 已存在申请时重复提交覆盖原申请，仍只有一份。
	extRepo.findByClaimFn = func(claimID uint64) (*model.WishExtension, error) {
		return &model.WishExtension{ID: 3, ClaimID: 9, WishID: 5, UserID: 2, Status: constants.ExtensionStatusRejected}, nil
	}
	ext2, err := svc.RequestExtension(context.Background(), 2, 9, dto.RequestExtensionRequest{NewDeadline: newDeadline, Reason: "再试一次"}, "127.0.0.1", "req-10")
	if err != nil {
		t.Fatalf("resubmit extension: %v", err)
	}
	if ext2.ID != 3 || ext2.Status != constants.ExtensionStatusPending || ext2.Reason != "再试一次" {
		t.Fatalf("expected single upserted pending extension, got %+v", ext2)
	}
}

// 发布者同意延期：截止日更新、进度与里程碑保留、心愿回到圆梦中。
func TestWishClaimService_ApproveExtension(t *testing.T) {
	t.Parallel()
	oldDeadline := time.Now().Add(24 * time.Hour)
	newDeadline := time.Now().Add(8 * 24 * time.Hour)
	var savedWish *model.Wish
	wishRepo := &mockWishRepo{
		findByIDFn: func(id uint64) (*model.Wish, error) {
			return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusExtensionPending, ExpectedDeadline: &oldDeadline}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { savedWish = wish; return nil },
	}
	claimRepo := &mockClaimRepo{
		findByIDFn: func(id uint64) (*model.WishClaim, error) {
			return &model.WishClaim{ID: id, WishID: 5, UserID: 2, Progress: 60, MilestoneCount: 2, Status: constants.WishStatusInProgress}, nil
		},
	}
	extRepo := &mockExtRepo{
		findByClaimFn: func(claimID uint64) (*model.WishExtension, error) {
			return &model.WishExtension{ID: 3, ClaimID: 9, WishID: 5, UserID: 2, NewDeadline: newDeadline, Status: constants.ExtensionStatusPending}, nil
		},
	}
	svc := NewWishClaimService(&mockTx{}, wishRepo, claimRepo, extRepo, &mockUserRepo{}, &mockBadge{}, &mockAudit{}, testLogger())

	// 非发布者无权裁决。
	_, err := svc.ApproveExtension(context.Background(), 2, 9, "127.0.0.1", "req-11")
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Code != constants.CodeWishNotOwner {
		t.Fatalf("expected CodeWishNotOwner, got %v", err)
	}

	ext, err := svc.ApproveExtension(context.Background(), 1, 9, "127.0.0.1", "req-12")
	if err != nil {
		t.Fatalf("approve extension: %v", err)
	}
	if ext.Status != constants.ExtensionStatusApproved {
		t.Fatalf("expected approved, got %s", ext.Status)
	}
	if savedWish == nil || savedWish.ExpectedDeadline == nil || !savedWish.ExpectedDeadline.Equal(newDeadline) {
		t.Fatalf("expected deadline updated to %v, got %+v", newDeadline, savedWish)
	}
	if savedWish.Status != constants.WishStatusInProgress {
		t.Fatalf("expected wish back to in_progress, got %s", savedWish.Status)
	}
}

// 发布者拒绝延期：原截止日不变，心愿回到圆梦中。
func TestWishClaimService_RejectExtension(t *testing.T) {
	t.Parallel()
	oldDeadline := time.Now().Add(24 * time.Hour)
	newDeadline := time.Now().Add(8 * 24 * time.Hour)
	var savedWish *model.Wish
	wishRepo := &mockWishRepo{
		findByIDFn: func(id uint64) (*model.Wish, error) {
			return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusExtensionPending, ExpectedDeadline: &oldDeadline}, nil
		},
		updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { savedWish = wish; return nil },
	}
	claimRepo := &mockClaimRepo{
		findByIDFn: func(id uint64) (*model.WishClaim, error) {
			return &model.WishClaim{ID: id, WishID: 5, UserID: 2, Progress: 60, Status: constants.WishStatusInProgress}, nil
		},
	}
	extRepo := &mockExtRepo{
		findByClaimFn: func(claimID uint64) (*model.WishExtension, error) {
			return &model.WishExtension{ID: 3, ClaimID: 9, WishID: 5, UserID: 2, NewDeadline: newDeadline, Status: constants.ExtensionStatusPending}, nil
		},
	}
	svc := NewWishClaimService(&mockTx{}, wishRepo, claimRepo, extRepo, &mockUserRepo{}, &mockBadge{}, &mockAudit{}, testLogger())

	ext, err := svc.RejectExtension(context.Background(), 1, 9, "127.0.0.1", "req-13")
	if err != nil {
		t.Fatalf("reject extension: %v", err)
	}
	if ext.Status != constants.ExtensionStatusRejected {
		t.Fatalf("expected rejected, got %s", ext.Status)
	}
	if savedWish == nil || savedWish.ExpectedDeadline == nil || !savedWish.ExpectedDeadline.Equal(oldDeadline) {
		t.Fatalf("expected original deadline kept, got %+v", savedWish)
	}
	if savedWish.Status != constants.WishStatusInProgress {
		t.Fatalf("expected wish back to in_progress, got %s", savedWish.Status)
	}

	// 已处理的申请不能重复裁决。
	extRepo.findByClaimFn = func(claimID uint64) (*model.WishExtension, error) {
		return &model.WishExtension{ID: 3, ClaimID: 9, WishID: 5, UserID: 2, NewDeadline: newDeadline, Status: constants.ExtensionStatusRejected}, nil
	}
	_, err = svc.RejectExtension(context.Background(), 1, 9, "127.0.0.1", "req-14")
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionInvalid {
		t.Fatalf("expected CodeExtensionInvalid, got %v", err)
	}
}
