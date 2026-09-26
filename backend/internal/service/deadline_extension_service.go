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

// DeadlineExtensionService 截止日延期协商服务：圆梦人申请（upsert，同一条申请只保留一份）、
// 发布者审核（同意更新截止日 / 拒绝保持原日期）、详情查询（按身份返回可执行操作）。
type DeadlineExtensionService interface {
	Apply(ctx context.Context, userID, wishID uint64, req dto.ApplyExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error)
	Review(ctx context.Context, userID, wishID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error)
	GetByWishID(userID uint64, wishID uint64) (*dto.ExtensionResponse, error)
}

type deadlineExtensionService struct {
	tx        repository.TxManager
	wish      repository.WishRepository
	claim     repository.WishClaimRepository
	extension repository.DeadlineExtensionRepository
	user      repository.UserRepository
	audit     AuditService
	logger    *slog.Logger
}

// NewDeadlineExtensionService 构造延期协商服务。
func NewDeadlineExtensionService(
	tx repository.TxManager,
	wish repository.WishRepository,
	claim repository.WishClaimRepository,
	extension repository.DeadlineExtensionRepository,
	user repository.UserRepository,
	audit AuditService,
	logger *slog.Logger,
) DeadlineExtensionService {
	return &deadlineExtensionService{tx: tx, wish: wish, claim: claim, extension: extension, user: user, audit: audit, logger: logger}
}

// Apply 圆梦人提交延期协商：行锁锁定心愿与申请，申请为 upsert（同一条申请只保留一份）。
func (s *deadlineExtensionService) Apply(ctx context.Context, userID, wishID uint64, req dto.ApplyExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error) {
	var saved *model.DeadlineExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		wish, err := s.wish.FindByIDForUpdate(tx, wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		claim, err := s.claim.FindByWishID(wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeExtensionNotAllowed, "心愿还没有圆梦人，无法申请延期", errors.New("claim not found"))
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if claim.UserID != userID {
			return util.NewAppError(constants.CodeClaimNotOwner, "只有圆梦人（用户 "+u64str(userID)+"）才能申请延期", errors.New("claim owner mismatch"))
		}
		if claim.Status == constants.WishStatusCompleted || wish.Status == constants.WishStatusCompleted {
			return util.NewAppError(constants.CodeExtensionNotAllowed, "心愿已完成，无需再申请延期", errors.New("wish already completed"))
		}
		// 同一认领只保留一份申请：无论待处理还是已拒绝后再次提交，都更新同一行（不新增）。
		if existing, eerr := s.extension.FindByWishIDForUpdate(tx, wishID); eerr == nil {
			if existing.Status == constants.ExtensionStatusApproved {
				return util.NewAppError(constants.CodeExtensionReviewed, "延期已同意，截止日已更新，不能重复申请", errors.New("extension approved"))
			}
			if err := validateExtensionRequest(wish.ExpectedDeadline, req.NewDeadline); err != nil {
				return err
			}
			existing.NewDeadline = req.NewDeadline
			existing.Reason = req.Reason
			existing.Status = constants.ExtensionStatusPending
			existing.ReviewerID = 0
			existing.ReviewComment = ""
			existing.ReviewedAt = nil
			if err := s.extension.UpdateWithTx(tx, existing); err != nil {
				return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
			}
			saved = existing
		} else if errors.Is(eerr, repository.ErrNotFound) {
			if err := validateExtensionRequest(wish.ExpectedDeadline, req.NewDeadline); err != nil {
				return err
			}
			extension := &model.DeadlineExtension{
				WishID:           wishID,
				ClaimID:          claim.ID,
				UserID:           userID,
				OriginalDeadline: wish.ExpectedDeadline,
				NewDeadline:      req.NewDeadline,
				Reason:           req.Reason,
				Status:           constants.ExtensionStatusPending,
			}
			if err := s.extension.CreateWithTx(tx, extension); err != nil {
				if errors.Is(err, repository.ErrConflict) {
					return util.NewAppError(constants.CodeExtensionPending, constants.MsgExtensionPending, err)
				}
				return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
			}
			saved = extension
		} else {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, eerr)
		}

		// 心愿进入延期待处理；圆梦认领状态保持不变（进度与里程碑继续保留）。
		wish.Status = constants.WishStatusExtensionPending
		if err := s.wish.UpdateWithTx(tx, wish); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogExtensionApplied, "wish_id", wishID, "extension_id", saved.ID, "user_id", userID, "new_deadline", saved.NewDeadline.Format("2006-01-02"))
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: "apply_extension", EntityType: "deadline_extension", EntityID: u64str(saved.ID),
		Detail: "圆梦人申请延期至 " + saved.NewDeadline.Format("2006-01-02") + "：" + util.TruncateString(saved.Reason, 50),
		IP:     ip, RequestID: requestID,
	})
	return saved, nil
}

// validateExtensionRequest 校验新截止日：必须晚于当前时间，且晚于当前生效的截止日。
func validateExtensionRequest(current *time.Time, next time.Time) error {
	if !next.After(nowFunc()) {
		return util.NewAppError(constants.CodeExtensionDeadlineInvalid, constants.MsgExtensionDeadlineInvalid+"：新截止日不能早于当前时间", errors.New("new deadline in the past"))
	}
	if current != nil && !next.After(*current) {
		return util.NewAppError(constants.CodeExtensionDeadlineInvalid, constants.MsgExtensionDeadlineInvalid, errors.New("new deadline not after current"))
	}
	return nil
}

// Review 发布者审核延期申请：同意则更新心愿截止日并回到原圆梦状态；拒绝则原日期不变。
func (s *deadlineExtensionService) Review(ctx context.Context, userID, wishID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error) {
	var saved *model.DeadlineExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		wish, err := s.wish.FindByIDForUpdate(tx, wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if wish.UserID != userID {
			return util.NewAppError(constants.CodeExtensionNotReviewer, constants.MsgExtensionNotReviewer, errors.New("wish publisher mismatch"))
		}
		extension, err := s.extension.FindByWishIDForUpdate(tx, wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeExtensionNotFound, constants.MsgExtensionNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if extension.Status != constants.ExtensionStatusPending {
			return util.NewAppError(constants.CodeExtensionReviewed, constants.MsgExtensionReviewed+"（当前状态："+constants.ExtensionStatusText(extension.Status)+"）", errors.New("extension already reviewed"))
		}

		reviewedAt := nowFunc()
		extension.ReviewerID = userID
		extension.ReviewComment = req.Comment
		extension.ReviewedAt = &reviewedAt

		// 审核结束后心愿恢复圆梦状态：认领进度 >0 为圆梦中，否则为已认领；进度与里程碑原样保留。
		claim, cerr := s.claim.FindByWishID(wishID)
		claimStatus := constants.WishStatusClaimed
		if cerr == nil && claim.Progress > 0 {
			claimStatus = constants.WishStatusInProgress
		}

		if req.Approved {
			// 同意：更新截止日，回到圆梦中状态；认领进度与里程碑原样保留。
			deadline := extension.NewDeadline
			wish.ExpectedDeadline = &deadline
			extension.Status = constants.ExtensionStatusApproved
			wish.Status = claimStatus
		} else {
			// 拒绝：原截止日不变，同样恢复圆梦状态。
			extension.Status = constants.ExtensionStatusRejected
			wish.Status = claimStatus
		}

		if err := s.extension.UpdateWithTx(tx, extension); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if err := s.wish.UpdateWithTx(tx, wish); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		saved = extension
		return nil
	})
	if err != nil {
		return nil, err
	}
	action := constants.LogExtensionRejected
	auditAction := "reject_extension"
	detail := "发布者拒绝延期，原截止日不变"
	if req.Approved {
		action = constants.LogExtensionApproved
		auditAction = "approve_extension"
		detail = "发布者同意延期，截止日更新为 " + saved.NewDeadline.Format("2006-01-02")
	}
	s.logger.Info(action, "wish_id", wishID, "extension_id", saved.ID, "reviewer_id", userID)
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: auditAction, EntityType: "deadline_extension", EntityID: u64str(saved.ID),
		Detail: detail, IP: ip, RequestID: requestID,
	})
	return saved, nil
}

// GetByWishID 查询某心愿的延期申请（详情页圆梦人/发布者模块复用；其他人只返回状态概览）。
func (s *deadlineExtensionService) GetByWishID(userID, wishID uint64) (*dto.ExtensionResponse, error) {
	extension, err := s.extension.FindByWishID(wishID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeExtensionNotFound, constants.MsgExtensionNotFound, err)
	}
	name := ""
	if fulfiller, ferr := s.user.FindByID(extension.UserID); ferr == nil {
		name = fulfiller.Nickname
	}
	resp := dto.ToExtensionResponse(extension, name)
	return &resp, nil
}
