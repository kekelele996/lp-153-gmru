package constants

// 心愿状态机：pending -> claimed -> in_progress -> completed；
// 圆梦人提交延期协商后进入 extension_pending（延期待处理），发布者同意/拒绝后回到原圆梦状态。
const (
	WishStatusPending          = "pending"
	WishStatusClaimed          = "claimed"
	WishStatusInProgress       = "in_progress"
	WishStatusCompleted        = "completed"
	WishStatusExtensionPending = "extension_pending"
)

// ValidWishStatuses 状态筛选白名单。
var ValidWishStatuses = []string{
	WishStatusPending,
	WishStatusClaimed,
	WishStatusInProgress,
	WishStatusCompleted,
	WishStatusExtensionPending,
}

// WishStatusText 状态文本（formatters 亦引用）。
func WishStatusText(status string) string {
	switch status {
	case WishStatusPending:
		return "待认领"
	case WishStatusClaimed:
		return "已被认领"
	case WishStatusInProgress:
		return "圆梦中"
	case WishStatusCompleted:
		return "已完成"
	case WishStatusExtensionPending:
		return "延期待处理"
	default:
		return "未知"
	}
}

// IsActiveFulfillmentStatus 该状态是否表示已有圆梦人（认领后，含延期协商中）。
func IsActiveFulfillmentStatus(status string) bool {
	switch status {
	case WishStatusClaimed, WishStatusInProgress, WishStatusExtensionPending:
		return true
	default:
		return false
	}
}
