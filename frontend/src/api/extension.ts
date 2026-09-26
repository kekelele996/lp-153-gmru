import { http } from "@/utils/request";

// 截止日延期协商：圆梦人申请、发布者审核、详情页按身份查询完整申请。
export interface DeadlineExtension {
  id: number;
  wish_id: number;
  claim_id: number;
  user_id: number;
  fulfiller_name?: string;
  original_deadline?: string | null;
  new_deadline: string;
  reason: string;
  status: string;
  reviewer_id: number;
  review_comment?: string;
  reviewed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ApplyExtensionPayload {
  new_deadline: string;
  reason: string;
}

export interface ReviewExtensionPayload {
  approved: boolean;
  comment?: string;
}

export const extensionApi = {
  apply: (wishId: number, payload: ApplyExtensionPayload) =>
    http.post<DeadlineExtension>(`/wishes/${wishId}/extension`, payload),
  review: (wishId: number, payload: ReviewExtensionPayload) =>
    http.post<DeadlineExtension>(`/wishes/${wishId}/extension/review`, payload),
  getByWish: (wishId: number) =>
    http.get<DeadlineExtension>(`/wishes/${wishId}/extension`),
};
