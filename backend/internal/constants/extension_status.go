package constants

// 延期协商状态机：圆梦人提交后为 pending；发布者同意 -> approved，拒绝 -> rejected。
// rejected 后圆梦人可就同一条申请再次提交，状态重新回到 pending（同一条申请只保留一份）。
const (
	ExtensionStatusPending  = "pending"
	ExtensionStatusApproved = "approved"
	ExtensionStatusRejected = "rejected"
)

// ValidExtensionStatuses 延期状态白名单。
var ValidExtensionStatuses = []string{
	ExtensionStatusPending,
	ExtensionStatusApproved,
	ExtensionStatusRejected,
}

// ExtensionStatusText 状态文本（formatters 亦引用）。
func ExtensionStatusText(status string) string {
	switch status {
	case ExtensionStatusPending:
		return "延期待处理"
	case ExtensionStatusApproved:
		return "延期已同意"
	case ExtensionStatusRejected:
		return "延期已拒绝"
	default:
		return "未知"
	}
}
