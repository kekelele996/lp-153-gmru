import { useState } from "react";
import type { WishDetail } from "@/api/wish";
import { extensionApi } from "@/api/extension";
import ConfirmDialog from "@/components/ConfirmDialog";
import { useToast } from "@/components/Toast";
import { EXTENSION_STATUS, EXTENSION_STATUS_STYLE, EXTENSION_STATUS_TEXT } from "@/constants";
import { useAuth } from "@/hooks/useAuth";
import { formatDate, formatDeadline } from "@/utils/format";

interface ExtensionPanelProps {
  wish: WishDetail;
  onChanged: () => void;
}

// 延期协商面板：详情页按双方身份展示申请状态和操作入口，其他人只看到脱敏状态。
export default function ExtensionPanel({ wish, onChanged }: ExtensionPanelProps) {
  const { user } = useAuth();
  const toast = useToast();
  const extension = wish.extension ?? null;
  const [newDeadline, setNewDeadline] = useState("");
  const [reason, setReason] = useState(extension?.status === EXTENSION_STATUS.PENDING ? extension.reason : "");
  const [submitting, setSubmitting] = useState(false);
  const [confirmReject, setConfirmReject] = useState(false);

  const isOwner = Boolean(user && wish.user_id === user.id);
  const isFulfiller = Boolean(user && wish.claim && wish.claim.user_id === user.id);
  const isParty = isOwner || isFulfiller;
  const completed = wish.status === "completed";

  // 其他人（含未登录）：只能查看状态概览。
  if (!isParty) {
    if (!wish.extension_brief) return null;
    return (
      <div className="rounded-xl border border-orange-100 bg-orange-50/60 p-4 text-sm text-orange-700">
        ⏳ 该心愿的截止日延期协商：{wish.extension_brief.text}
      </div>
    );
  }

  const status = extension?.status;

  const submit = async () => {
    if (!newDeadline) {
      toast.show("请选择新的截止日", "error");
      return;
    }
    if (reason.trim().length < 2) {
      toast.show("请填写延期原因（至少 2 个字）", "error");
      return;
    }
    setSubmitting(true);
    try {
      await extensionApi.apply(wish.id, { new_deadline: `${newDeadline}T23:59:59+08:00`, reason: reason.trim() });
      toast.show("延期申请已提交，等待发布者处理 ⏳");
      onChanged();
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  };

  const review = async (approved: boolean, comment?: string) => {
    setSubmitting(true);
    try {
      await extensionApi.review(wish.id, { approved, comment });
      toast.show(approved ? "已同意延期，截止日已更新 ✅" : "已拒绝，截止日保持不变");
      setConfirmReject(false);
      onChanged();
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-3 rounded-xl border border-orange-200 bg-orange-50/70 p-4">
      <div className="flex items-center justify-between">
        <p className="text-sm font-semibold text-orange-700">⏳ 截止日延期协商</p>
        {status && (
          <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${EXTENSION_STATUS_STYLE[status] || "bg-gray-100 text-gray-600"}`}>
            {EXTENSION_STATUS_TEXT[status] || status}
          </span>
        )}
      </div>

      {extension && (
        <div className="space-y-1 rounded-lg bg-white/80 p-3 text-sm text-gray-700">
          <p>
            圆梦人申请：<span className="font-medium">{formatDeadline(wish.expected_deadline)}</span>
            {" → "}
            <span className="font-medium text-orange-700">{formatDeadline(extension.new_deadline)}</span>
            <span className="ml-2 text-xs text-gray-400">提交于 {formatDate(extension.created_at)}</span>
          </p>
          <p className="whitespace-pre-wrap text-gray-600">原因：{extension.reason}</p>
          {extension.review_comment && <p className="text-gray-500">发布者回复：{extension.review_comment}</p>}
          {extension.reviewed_at && <p className="text-xs text-gray-400">审核于 {formatDate(extension.reviewed_at)}</p>}
        </div>
      )}

      {/* 圆梦人操作入口：待处理可修改重提、被拒后可再次申请；已同意不可再申请 */}
      {isFulfiller && !completed && status !== EXTENSION_STATUS.APPROVED && (
        <div className="space-y-2 rounded-lg bg-white p-3">
          <p className="text-xs text-gray-500">
            {status === EXTENSION_STATUS.PENDING
              ? "申请待发布者处理，你可以修改后重新提交（同一条申请只保留一份）"
              : status === EXTENSION_STATUS.REJECTED
                ? "发布者拒绝了上次申请，原截止日不变；你可以调整后再次申请"
                : "心愿快到截止日还没完成？向发布者申请延期"}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="date"
              className="input max-w-[200px]"
              value={newDeadline}
              onChange={(e) => setNewDeadline(e.target.value)}
            />
            <span className="text-xs text-gray-400">新的截止日（须晚于当前截止日）</span>
          </div>
          <textarea
            className="input min-h-[60px]"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="说明延期原因，让发布者更安心..."
          />
          <button className="btn-primary" disabled={submitting} onClick={submit}>
            {submitting ? "提交中..." : status === EXTENSION_STATUS.PENDING ? "更新延期申请" : "提交延期申请"}
          </button>
        </div>
      )}

      {isFulfiller && status === EXTENSION_STATUS.APPROVED && !completed && (
        <p className="text-sm text-emerald-700">✅ 发布者已同意延期，请在新截止日前完成心愿。</p>
      )}

      {/* 发布者操作入口：仅在待处理时展示同意 / 拒绝 */}
      {isOwner && status === EXTENSION_STATUS.PENDING && (
        <div className="flex gap-3">
          <button className="btn-primary" disabled={submitting} onClick={() => review(true)}>
            同意延期（更新截止日）
          </button>
          <button className="btn-secondary" disabled={submitting} onClick={() => setConfirmReject(true)}>
            拒绝（原日期不变）
          </button>
        </div>
      )}
      {isOwner && status === EXTENSION_STATUS.APPROVED && (
        <p className="text-sm text-emerald-700">✅ 你已同意延期，截止日已更新，进度与里程碑继续保留。</p>
      )}
      {isOwner && status === EXTENSION_STATUS.REJECTED && (
        <p className="text-sm text-rose-700">已拒绝该申请，原截止日保持不变；圆梦人可调整后再次申请。</p>
      )}

      <ConfirmDialog
        open={confirmReject}
        title="拒绝延期申请"
        description="拒绝后心愿截止日保持不变，圆梦人可以调整方案后再次申请。确认拒绝吗？"
        confirmText="确认拒绝"
        onConfirm={() => review(false)}
        onCancel={() => setConfirmReject(false)}
      />
    </div>
  );
}
