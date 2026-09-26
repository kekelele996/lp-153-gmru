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

func newExtensionService(wishRepo *mockWishRepo, claimRepo *mockClaimRepo, extRepo *mockExtensionRepo) *deadlineExtensionService {
	return NewDeadlineExtensionService(&mockTx{}, wishRepo, claimRepo, extRepo, &mockUserRepo{}, &mockAudit{}, testLogger()).(*deadlineExtensionService)
}

func TestDeadlineExtensionService_Apply(t *testing.T) {
	t.Parallel()
	current := time.Now().Add(24 * time.Hour)
	next := current.Add(72 * time.Hour)

	t.Run("apply success creates single extension and wish enters extension_pending", func(t *testing.T) {
		t.Parallel()
		var savedStatus string
		wishRepo := &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress, ExpectedDeadline: &current}, nil
			},
			updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { savedStatus = wish.Status; return nil },
		}
		claimRepo := &mockClaimRepo{
			findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
				return &model.WishClaim{ID: 3, WishID: wishID, UserID: 2, Progress: 40, Status: constants.WishStatusInProgress}, nil
			},
		}
		var created *model.DeadlineExtension
		extRepo := newNotFoundExtensionRepo()
		extRepo.createWithTxFn = func(tx *gorm.DB, e *model.DeadlineExtension) error { e.ID = 9; created = e; return nil }

		svc := newExtensionService(wishRepo, claimRepo, extRepo)
		ext, err := svc.Apply(context.Background(), 2, 5, dto.ApplyExtensionRequest{NewDeadline: next, Reason: "考试周，需要再缓几天"}, "127.0.0.1", "req-1")
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if ext.Status != constants.ExtensionStatusPending || created.ClaimID != 3 {
			t.Fatalf("unexpected extension %+v", ext)
		}
		if savedStatus != constants.WishStatusExtensionPending {
			t.Fatalf("wish status = %s, want extension_pending", savedStatus)
		}
	})

	t.Run("resubmit updates the same row instead of creating", func(t *testing.T) {
		t.Parallel()
		rejected := &model.DeadlineExtension{ID: 9, WishID: 5, ClaimID: 3, UserID: 2, Status: constants.ExtensionStatusRejected, NewDeadline: next}
		wishRepo := &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress, ExpectedDeadline: &current}, nil
			},
			updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error { return nil },
		}
		claimRepo := &mockClaimRepo{
			findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
				return &model.WishClaim{ID: 3, WishID: wishID, UserID: 2, Progress: 40, Status: constants.WishStatusInProgress}, nil
			},
		}
		updated := false
		created := false
		extRepo := &mockExtensionRepo{
			findByWishForUpdFn: func(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) { return rejected, nil },
			updateWithTxFn:     func(tx *gorm.DB, e *model.DeadlineExtension) error { updated = true; return nil },
			createWithTxFn:     func(tx *gorm.DB, e *model.DeadlineExtension) error { created = true; return nil },
		}
		svc := newExtensionService(wishRepo, claimRepo, extRepo)
		ext, err := svc.Apply(context.Background(), 2, 5, dto.ApplyExtensionRequest{NewDeadline: next.Add(24 * time.Hour), Reason: "再次申请"}, "127.0.0.1", "req-2")
		if err != nil {
			t.Fatalf("reapply: %v", err)
		}
		if !updated || created {
			t.Fatal("reapply must update the existing row, not create")
		}
		if ext.ID != 9 || ext.Status != constants.ExtensionStatusPending {
			t.Fatalf("unexpected extension id=%d status=%s", ext.ID, ext.Status)
		}
	})

	t.Run("non-fulfiller forbidden", func(t *testing.T) {
		t.Parallel()
		wishRepo := &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress, ExpectedDeadline: &current}, nil
			},
		}
		claimRepo := &mockClaimRepo{
			findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
				return &model.WishClaim{ID: 3, WishID: wishID, UserID: 2}, nil
			},
		}
		svc := newExtensionService(wishRepo, claimRepo, newNotFoundExtensionRepo())
		_, err := svc.Apply(context.Background(), 8, 5, dto.ApplyExtensionRequest{NewDeadline: next, Reason: "越权申请"}, "127.0.0.1", "req-3")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeClaimNotOwner {
			t.Fatalf("expected CodeClaimNotOwner, got %v", err)
		}
	})

	t.Run("deadline not after current rejected", func(t *testing.T) {
		t.Parallel()
		wishRepo := &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress, ExpectedDeadline: &current}, nil
			},
		}
		claimRepo := &mockClaimRepo{
			findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
				return &model.WishClaim{ID: 3, WishID: wishID, UserID: 2}, nil
			},
		}
		svc := newExtensionService(wishRepo, claimRepo, newNotFoundExtensionRepo())
		_, err := svc.Apply(context.Background(), 2, 5, dto.ApplyExtensionRequest{NewDeadline: current, Reason: "日期没变"}, "127.0.0.1", "req-4")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionDeadlineInvalid {
			t.Fatalf("expected CodeExtensionDeadlineInvalid, got %v", err)
		}
	})

	t.Run("approved extension cannot be reapplied", func(t *testing.T) {
		t.Parallel()
		approved := &model.DeadlineExtension{ID: 9, WishID: 5, ClaimID: 3, UserID: 2, Status: constants.ExtensionStatusApproved, NewDeadline: next}
		wishRepo := &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress, ExpectedDeadline: &next}, nil
			},
		}
		claimRepo := &mockClaimRepo{
			findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
				return &model.WishClaim{ID: 3, WishID: wishID, UserID: 2}, nil
			},
		}
		extRepo := &mockExtensionRepo{
			findByWishForUpdFn: func(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) { return approved, nil },
		}
		svc := newExtensionService(wishRepo, claimRepo, extRepo)
		_, err := svc.Apply(context.Background(), 2, 5, dto.ApplyExtensionRequest{NewDeadline: next.Add(48 * time.Hour), Reason: "再延一次"}, "127.0.0.1", "req-5")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionReviewed {
			t.Fatalf("expected CodeExtensionReviewed, got %v", err)
		}
	})
}

func TestDeadlineExtensionService_Review(t *testing.T) {
	t.Parallel()
	current := time.Now().Add(24 * time.Hour)
	next := current.Add(72 * time.Hour)

	newWishes := func(status string) (*mockWishRepo, *model.Wish) {
		w := &model.Wish{ID: 5, UserID: 1, Status: status, ExpectedDeadline: &current}
		return &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) { return w, nil },
			updateWithTxFn:      func(tx *gorm.DB, wish *model.Wish) error { return nil },
		}, w
	}
	newClaim := func(progress int) *mockClaimRepo {
		return &mockClaimRepo{
			findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
				return &model.WishClaim{ID: 3, WishID: wishID, UserID: 2, Progress: progress, Status: constants.WishStatusInProgress}, nil
			},
		}
	}

	t.Run("approve updates deadline and restores in_progress", func(t *testing.T) {
		t.Parallel()
		wishRepo, wish := newWishes(constants.WishStatusExtensionPending)
		pending := &model.DeadlineExtension{ID: 9, WishID: 5, ClaimID: 3, UserID: 2, Status: constants.ExtensionStatusPending, OriginalDeadline: &current, NewDeadline: next}
		extRepo := &mockExtensionRepo{
			findByWishForUpdFn: func(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) { return pending, nil },
			updateWithTxFn:     func(tx *gorm.DB, e *model.DeadlineExtension) error { return nil },
		}
		svc := newExtensionService(wishRepo, newClaim(60), extRepo)
		ext, err := svc.Review(context.Background(), 1, 5, dto.ReviewExtensionRequest{Approved: true, Comment: "同意"}, "127.0.0.1", "req-6")
		if err != nil {
			t.Fatalf("review: %v", err)
		}
		if ext.Status != constants.ExtensionStatusApproved {
			t.Fatalf("extension status = %s", ext.Status)
		}
		if !wish.ExpectedDeadline.Equal(next) {
			t.Fatalf("deadline = %v, want %v", wish.ExpectedDeadline, next)
		}
		if wish.Status != constants.WishStatusInProgress {
			t.Fatalf("wish status = %s, want in_progress", wish.Status)
		}
	})

	t.Run("reject keeps original deadline", func(t *testing.T) {
		t.Parallel()
		wishRepo, wish := newWishes(constants.WishStatusExtensionPending)
		pending := &model.DeadlineExtension{ID: 9, WishID: 5, ClaimID: 3, UserID: 2, Status: constants.ExtensionStatusPending, OriginalDeadline: &current, NewDeadline: next}
		extRepo := &mockExtensionRepo{
			findByWishForUpdFn: func(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) { return pending, nil },
			updateWithTxFn:     func(tx *gorm.DB, e *model.DeadlineExtension) error { return nil },
		}
		svc := newExtensionService(wishRepo, newClaim(0), extRepo)
		ext, err := svc.Review(context.Background(), 1, 5, dto.ReviewExtensionRequest{Approved: false}, "127.0.0.1", "req-7")
		if err != nil {
			t.Fatalf("review: %v", err)
		}
		if ext.Status != constants.ExtensionStatusRejected {
			t.Fatalf("extension status = %s", ext.Status)
		}
		if !wish.ExpectedDeadline.Equal(current) {
			t.Fatalf("deadline changed on reject: %v", wish.ExpectedDeadline)
		}
		if wish.Status != constants.WishStatusClaimed {
			t.Fatalf("wish status = %s, want claimed", wish.Status)
		}
	})

	t.Run("non-publisher forbidden", func(t *testing.T) {
		t.Parallel()
		wishRepo, _ := newWishes(constants.WishStatusExtensionPending)
		svc := newExtensionService(wishRepo, newClaim(10), newNotFoundExtensionRepo())
		_, err := svc.Review(context.Background(), 2, 5, dto.ReviewExtensionRequest{Approved: true}, "127.0.0.1", "req-8")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionNotReviewer {
			t.Fatalf("expected CodeExtensionNotReviewer, got %v", err)
		}
	})

	t.Run("already reviewed cannot review again", func(t *testing.T) {
		t.Parallel()
		wishRepo, _ := newWishes(constants.WishStatusInProgress)
		approved := &model.DeadlineExtension{ID: 9, WishID: 5, ClaimID: 3, UserID: 2, Status: constants.ExtensionStatusApproved}
		extRepo := &mockExtensionRepo{
			findByWishForUpdFn: func(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) { return approved, nil },
		}
		svc := newExtensionService(wishRepo, newClaim(10), extRepo)
		_, err := svc.Review(context.Background(), 1, 5, dto.ReviewExtensionRequest{Approved: false}, "127.0.0.1", "req-9")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionReviewed {
			t.Fatalf("expected CodeExtensionReviewed, got %v", err)
		}
	})

	t.Run("not found returns extension not found", func(t *testing.T) {
		t.Parallel()
		wishRepo, _ := newWishes(constants.WishStatusExtensionPending)
		svc := newExtensionService(wishRepo, newClaim(10), newNotFoundExtensionRepo())
		_, err := svc.Review(context.Background(), 1, 5, dto.ReviewExtensionRequest{Approved: true}, "127.0.0.1", "req-10")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionNotFound {
			t.Fatalf("expected CodeExtensionNotFound, got %v", err)
		}
	})
}
