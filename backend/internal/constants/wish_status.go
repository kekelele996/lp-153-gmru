package constants

// 心愿状态机：pending -> claimed -> in_progress -> completed；延期协商期间进入 extension_pending。
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
		return "延期协商中"
	default:
		return "未知"
	}
}
