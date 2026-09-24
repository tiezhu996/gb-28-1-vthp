import { request, buildQuery } from '@/utils/request';
import type { PageResult, WrongBook } from '@/types';

// 错题本列表查询参数：repeated_only 只看反复错题，sort=wrong_count 按错误次数倒序
export interface WrongBookQuery {
  subject?: string;
  knowledge_point?: string;
  status?: string;
  repeated_only?: boolean;
  sort?: string;
  page?: number;
  page_size?: number;
}

export const wrongBookApi = {
  list(query: WrongBookQuery) {
    return request<PageResult<WrongBook>>(`/wrong-books${buildQuery({ ...query, repeated_only: query.repeated_only ? 'true' : undefined })}`);
  },
  add(data: { question_id: string; exam_id?: string; exam_record_id?: string; note?: string }) {
    return request<WrongBook>('/wrong-books', { method: 'POST', body: JSON.stringify(data) });
  },
  update(id: string, data: { status?: string; note?: string }) {
    return request<WrongBook>(`/wrong-books/${id}`, { method: 'PUT', body: JSON.stringify(data) });
  },
  remove(id: string) {
    return request<null>(`/wrong-books/${id}`, { method: 'DELETE' });
  },
};
