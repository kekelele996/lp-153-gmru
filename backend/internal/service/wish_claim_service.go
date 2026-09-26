package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/model"
	"github.com/wishwall/wishwall/internal/repository"
	"github.com/wishwall/wishwall/internal/util"
)

// WishClaimService 心愿认领服务：认领（事务+行锁）、进度更新、完成、延期协商、我的认领。
type WishClaimService interface {
	Claim(ctx context.Context, userID, wishID uint64, ip, requestID string) (*model.WishClaim, error)
	UpdateProgress(ctx context.Context, userID, claimID uint64, req dto.UpdateProgressRequest, ip, requestID string) (*model.WishClaim, error)
	Complete(ctx context.Context, userID, claimID uint64, req dto.CompleteClaimRequest, ip, requestID string) (*model.WishClaim, error)
	RequestExtension(ctx context.Context, userID, claimID uint64, req dto.RequestExtensionRequest, ip, requestID string) (*model.WishExtension, error)
	ApproveExtension(ctx context.Context, userID, claimID uint64, ip, requestID string) (*model.WishExtension, error)
	RejectExtension(ctx context.Context, userID, claimID uint64, ip, requestID string) (*model.WishExtension, error)
	ListMine(userID uint64, q dto.PageQuery) (*dto.PageResult, error)
	GetByWishID(userID, wishID uint64) (*model.WishClaim, error)
}

type wishClaimService struct {
	tx     repository.TxManager
	wish   repository.WishRepository
	claim  repository.WishClaimRepository
	ext    repository.WishExtensionRepository
	user   repository.UserRepository
	badge  BadgeService
	audit  AuditService
	logger *slog.Logger
}

// NewWishClaimService 构造认领服务。
func NewWishClaimService(
	tx repository.TxManager,
	wish repository.WishRepository,
	claim repository.WishClaimRepository,
	ext repository.WishExtensionRepository,
	user repository.UserRepository,
	badge BadgeService,
	audit AuditService,
	logger *slog.Logger,
) WishClaimService {
	return &wishClaimService{tx: tx, wish: wish, claim: claim, ext: ext, user: user, badge: badge, audit: audit, logger: logger}
}

// Claim 认领心愿：SELECT ... FOR UPDATE 锁定心愿行防止并发重复认领。
func (s *wishClaimService) Claim(ctx context.Context, userID, wishID uint64, ip, requestID string) (*model.WishClaim, error) {
	var created *model.WishClaim
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		wish, err := s.wish.FindByIDForUpdate(tx, wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if wish.UserID == userID {
			return util.NewAppError(constants.CodeWishStatusInvalid, "不能认领自己发布的心愿", errors.New("self claim forbidden"))
		}
		if wish.Status != constants.WishStatusPending {
			return util.NewAppError(constants.CodeWishAlreadyClaimed, constants.MsgWishAlreadyClaimed, errors.New("wish status not pending"))
		}
		claim := &model.WishClaim{
			WishID: wishID, UserID: userID,
			Progress: 0, Status: constants.WishStatusClaimed,
		}
		if err := s.claim.CreateWithTx(tx, claim); err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return util.NewAppError(constants.CodeWishAlreadyClaimed, constants.MsgWishAlreadyClaimed, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		wish.Status = constants.WishStatusClaimed
		if err := s.wish.UpdateWithTx(tx, wish); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		created = claim
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogWishClaimed, "wish_id", wishID, "claim_id", created.ID, "user_id", userID)
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: "claim_wish", EntityType: "wish_claim", EntityID: u64str(created.ID),
		Detail: "认领心愿，成为圆梦人", IP: ip, RequestID: requestID,
	})
	if err := s.badge.GrantFirstClaim(userID); err != nil {
		s.logger.Warn("grant first claim badge failed", "error", err)
	}
	return created, nil
}

// UpdateProgress 更新进度：progress>=100 时状态机流转为 completed。
func (s *wishClaimService) UpdateProgress(ctx context.Context, userID, claimID uint64, req dto.UpdateProgressRequest, ip, requestID string) (*model.WishClaim, error) {
	var updated *model.WishClaim
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		claim, err := s.claim.FindByID(claimID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeClaimNotFound, constants.MsgClaimNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if claim.UserID != userID {
			return util.NewAppError(constants.CodeClaimNotOwner, "只有圆梦人才能更新进度", errors.New("claim owner mismatch"))
		}
		wish, err := s.wish.FindByID(claim.WishID)
		if err != nil {
			return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
		}
		claim.Progress = req.Progress
		if req.Note != "" {
			claim.LatestNote = req.Note
		}
		if req.IsMilestone {
			claim.MilestoneCount++
		}
		if claim.Progress >= 100 {
			// 延期协商待处理期间禁止标记完成，需等待发布者裁决。
			if wish.Status == constants.WishStatusExtensionPending {
				return util.NewAppError(constants.CodeClaimStatusInvalid, "延期协商待处理，暂不能标记完成", errors.New("extension pending, complete blocked"))
			}
			claim.Status = constants.WishStatusCompleted
			wish.Status = constants.WishStatusCompleted
			wish.CompletionNote = req.Note
		} else {
			if claim.Status == constants.WishStatusClaimed {
				claim.Status = constants.WishStatusInProgress
			}
			// 延期协商期间保留 extension_pending 状态，进度更新不冲掉协商标记。
			if wish.Status != constants.WishStatusExtensionPending {
				wish.Status = constants.WishStatusInProgress
			}
		}
		if err := s.claim.UpdateWithTx(tx, claim); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if err := s.wish.UpdateWithTx(tx, wish); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		updated = claim
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogClaimProgress, "claim_id", claimID, "progress", updated.Progress, "user_id", userID)
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: "update_progress", EntityType: "wish_claim", EntityID: u64str(claimID),
		Detail: "更新圆梦进度至 " + itoa(updated.Progress) + "%", IP: ip, RequestID: requestID,
	})
	if updated.Status == constants.WishStatusCompleted {
		s.logger.Info(constants.LogClaimCompleted, "claim_id", claimID, "user_id", userID)
		_ = s.audit.Record(&model.AuditLog{
			UserID: userID, Action: "complete_wish", EntityType: "wish_claim", EntityID: u64str(claimID),
			Detail: "心愿达成，进入庆祝时刻", IP: ip, RequestID: requestID,
		})
		if err := s.badge.GrantCompletionBadges(userID); err != nil {
			s.logger.Warn("grant completion badge failed", "error", err)
		}
	}
	return updated, nil
}

// Complete 直接完成心愿（进度置 100）。
func (s *wishClaimService) Complete(ctx context.Context, userID, claimID uint64, req dto.CompleteClaimRequest, ip, requestID string) (*model.WishClaim, error) {
	return s.UpdateProgress(ctx, userID, claimID, dto.UpdateProgressRequest{Progress: 100, Note: req.Note, IsMilestone: true}, ip, requestID)
}

// RequestExtension 圆梦人提交延期申请：同一认领仅保留一条（重复提交覆盖原申请），心愿转入延期待处理。
func (s *wishClaimService) RequestExtension(ctx context.Context, userID, claimID uint64, req dto.RequestExtensionRequest, ip, requestID string) (*model.WishExtension, error) {
	if req.NewDeadline.Before(time.Now()) {
		return nil, util.NewAppError(constants.CodeExtensionInvalid, "新的截止日必须晚于当前时间", errors.New("new deadline not in future"))
	}
	var saved *model.WishExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		claim, err := s.claim.FindByID(claimID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeClaimNotFound, constants.MsgClaimNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if claim.UserID != userID {
			return util.NewAppError(constants.CodeClaimNotOwner, "只有圆梦人才能申请延期", errors.New("claim owner mismatch"))
		}
		wish, err := s.wish.FindByID(claim.WishID)
		if err != nil {
			return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
		}
		if wish.Status == constants.WishStatusCompleted || claim.Status == constants.WishStatusCompleted {
			return util.NewAppError(constants.CodeClaimStatusInvalid, "心愿已完成，无需申请延期", errors.New("wish already completed"))
		}
		// 同一认领只保留一份申请：已存在则覆盖为新申请并重新进入待处理。
		ext, err := s.ext.FindByClaimID(claimID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
			}
			ext = &model.WishExtension{ClaimID: claimID, WishID: claim.WishID, UserID: userID}
		}
		ext.NewDeadline = req.NewDeadline
		ext.Reason = req.Reason
		ext.Status = constants.ExtensionStatusPending
		if ext.ID == 0 {
			if err := s.ext.CreateWithTx(tx, ext); err != nil {
				if errors.Is(err, repository.ErrConflict) {
					return util.NewAppError(constants.CodeExtensionInvalid, "已存在延期申请，请勿重复提交", err)
				}
				return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
			}
		} else if err := s.ext.UpdateWithTx(tx, ext); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		wish.Status = constants.WishStatusExtensionPending
		if err := s.wish.UpdateWithTx(tx, wish); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		saved = ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogExtensionRequested, "claim_id", claimID, "wish_id", saved.WishID, "user_id", userID)
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: "request_extension", EntityType: "wish_extension", EntityID: u64str(saved.ID),
		Detail: "申请延期至 " + saved.NewDeadline.Format("2006-01-02"), IP: ip, RequestID: requestID,
	})
	return saved, nil
}

// ApproveExtension 发布者同意延期：更新心愿截止日，进度与里程碑保留，心愿回到圆梦流程。
func (s *wishClaimService) ApproveExtension(ctx context.Context, userID, claimID uint64, ip, requestID string) (*model.WishExtension, error) {
	return s.decideExtension(userID, claimID, true, ip, requestID)
}

// RejectExtension 发布者拒绝延期：原截止日不变，心愿回到圆梦流程。
func (s *wishClaimService) RejectExtension(ctx context.Context, userID, claimID uint64, ip, requestID string) (*model.WishExtension, error) {
	return s.decideExtension(userID, claimID, false, ip, requestID)
}

// decideExtension 发布者裁决延期申请（同意/拒绝共用事务流程）。
func (s *wishClaimService) decideExtension(userID, claimID uint64, approve bool, ip, requestID string) (*model.WishExtension, error) {
	var saved *model.WishExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		claim, err := s.claim.FindByID(claimID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeClaimNotFound, constants.MsgClaimNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		wish, err := s.wish.FindByID(claim.WishID)
		if err != nil {
			return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
		}
		if wish.UserID != userID {
			return util.NewAppError(constants.CodeWishNotOwner, constants.MsgWishNotOwner, errors.New("wish owner mismatch"))
		}
		ext, err := s.ext.FindByClaimID(claimID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeExtensionNotFound, constants.MsgExtensionNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if ext.Status != constants.ExtensionStatusPending {
			return util.NewAppError(constants.CodeExtensionInvalid, "该延期申请已处理", errors.New("extension already decided"))
		}
		if approve {
			ext.Status = constants.ExtensionStatusApproved
			wish.ExpectedDeadline = &ext.NewDeadline
		} else {
			ext.Status = constants.ExtensionStatusRejected
		}
		// 裁决后心愿回到圆梦流程：有进度则圆梦中，否则回到已认领。
		if claim.Progress > 0 {
			wish.Status = constants.WishStatusInProgress
		} else {
			wish.Status = constants.WishStatusClaimed
		}
		if err := s.ext.UpdateWithTx(tx, ext); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if err := s.wish.UpdateWithTx(tx, wish); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		saved = ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	if approve {
		s.logger.Info(constants.LogExtensionApproved, "claim_id", claimID, "user_id", userID)
		_ = s.audit.Record(&model.AuditLog{
			UserID: userID, Action: "approve_extension", EntityType: "wish_extension", EntityID: u64str(saved.ID),
			Detail: "同意延期，截止日更新为 " + saved.NewDeadline.Format("2006-01-02"), IP: ip, RequestID: requestID,
		})
	} else {
		s.logger.Info(constants.LogExtensionRejected, "claim_id", claimID, "user_id", userID)
		_ = s.audit.Record(&model.AuditLog{
			UserID: userID, Action: "reject_extension", EntityType: "wish_extension", EntityID: u64str(saved.ID),
			Detail: "拒绝延期申请，原截止日不变", IP: ip, RequestID: requestID,
		})
	}
	return saved, nil
}

func (s *wishClaimService) ListMine(userID uint64, q dto.PageQuery) (*dto.PageResult, error) {
	page, size := q.Normalize()
	total, err := s.claim.CountByUserID(userID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	claims, err := s.claim.ListByUserID(userID, (page-1)*size, size)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	items := make([]dto.WishClaimResponse, 0, len(claims))
	for i := range claims {
		wishTitle := ""
		if wish, werr := s.wish.FindByID(claims[i].WishID); werr == nil {
			wishTitle = wish.Title
		}
		name := ""
		if u, uerr := s.user.FindByID(userID); uerr == nil {
			name = u.Nickname
		}
		items = append(items, dto.ToWishClaimResponse(&claims[i], wishTitle, name))
	}
	return &dto.PageResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}

// GetByWishID 查询某心愿的认领（心愿详情页圆梦人模块复用）。
func (s *wishClaimService) GetByWishID(userID, wishID uint64) (*model.WishClaim, error) {
	claim, err := s.claim.FindByWishID(wishID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeClaimNotFound, constants.MsgClaimNotFound, err)
	}
	if claim.UserID != userID {
		return nil, util.NewAppError(constants.CodeForbidden, constants.MsgNeedLogin+"：无权限查看该认领", errors.New("claim visibility forbidden"))
	}
	return claim, nil
}
