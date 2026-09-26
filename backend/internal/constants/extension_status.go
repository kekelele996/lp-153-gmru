package constants

// 延期协商申请状态机：pending -> approved / rejected。
const (
	ExtensionStatusPending  = "pending"
	ExtensionStatusApproved = "approved"
	ExtensionStatusRejected = "rejected"
)

// ExtensionStatusText 延期申请状态文本。
func ExtensionStatusText(status string) string {
	switch status {
	case ExtensionStatusPending:
		return "待发布者处理"
	case ExtensionStatusApproved:
		return "已同意"
	case ExtensionStatusRejected:
		return "已拒绝"
	default:
		return "未知"
	}
}
