'use client';
import { useCallback, useEffect, useState } from 'react';
import { useWrongBookStore } from '@/stores/wrongBookStore';
import { DataTable, type Column } from '@/components/DataTable';
import { Pagination } from '@/components/Pagination';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { StatusBadge } from '@/components/StatusBadge';
import { formatDateTime } from '@/utils/format';
import type { WrongBook } from '@/types';

export default function WrongBookPage() {
  const { list, total, loading, fetch, update, remove } = useWrongBookStore();
  const [page, setPage] = useState(1);
  const [subject, setSubject] = useState('');
  const [status, setStatus] = useState('');
  const [repeatedOnly, setRepeatedOnly] = useState(false);
  const [confirmId, setConfirmId] = useState<string | null>(null);

  const reload = useCallback(() => {
    // 复习页固定按错误次数倒序：错误次数多的排在前面
    fetch({ subject, status, repeated_only: repeatedOnly || undefined, sort: 'wrong_count', page, page_size: 10 });
  }, [fetch, subject, status, repeatedOnly, page]);

  useEffect(() => {
    reload();
  }, [reload]);

  const columns: Column<WrongBook>[] = [
    { key: 'question_content', title: '题目', render: (w) => <span className="line-clamp-1 max-w-xs">{w.question_content}</span> },
    { key: 'subject', title: '学科', render: (w) => <span className="text-xs text-gray-500">{w.subject}</span> },
    { key: 'knowledge_points', title: '知识点', render: (w) => <span className="text-xs text-gray-500">{(w.knowledge_points ?? []).join('、')}</span> },
    { key: 'my_answer', title: '最近错答', render: (w) => <span className="text-xs">{w.my_answer || '-'}</span> },
    { key: 'correct_answer', title: '正确答案', render: (w) => <span className="text-xs text-green-600">{w.correct_answer}</span> },
    { key: 'wrong_count', title: '错误次数', render: (w) => (
        <StatusBadge text={`错 ${w.wrong_count} 次`} color={w.wrong_count > 1 ? 'red' : 'orange'} />
      ) },
    { key: 'status', title: '掌握状态', render: (w) => (
        <StatusBadge text={w.status === 'resolved' ? '已掌握' : '未掌握'} color={w.status === 'resolved' ? 'green' : 'orange'} />
      ) },
    { key: 'last_wrong_at', title: '最近出错', render: (w) => <span className="text-xs">{formatDateTime(w.last_wrong_at)}</span> },
    { key: 'created_at', title: '首次收录', render: (w) => <span className="text-xs">{formatDateTime(w.created_at)}</span> },
    { key: 'actions', title: '操作', render: (w) => (
        <div className="flex gap-2">
          {w.status === 'active' && (
            <button onClick={async () => { await update(w.id, { status: 'resolved' }); reload(); }}
              className="text-green-600 hover:underline">标记已掌握</button>
          )}
          <button onClick={() => setConfirmId(w.id)} className="text-red-600 hover:underline">移除</button>
        </div>
      ) },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-bold text-gray-800">我的错题本</h1>
        <div className="flex flex-wrap items-center gap-2">
          <input value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="按学科筛选…"
            className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm" />
          <select value={status} onChange={(e) => setStatus(e.target.value)}
            className="rounded-lg border border-gray-300 px-2 py-1.5 text-sm">
            <option value="">全部状态</option>
            <option value="active">未掌握</option>
            <option value="resolved">已掌握</option>
          </select>
          <label className="flex items-center gap-1.5 text-sm text-gray-600">
            <input type="checkbox" checked={repeatedOnly}
              onChange={(e) => { setRepeatedOnly(e.target.checked); setPage(1); }} />
            只看反复错题
          </label>
          <button onClick={() => setPage(1)} className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50">查询</button>
        </div>
      </div>
      <DataTable columns={columns} rows={list} loading={loading} emptyTitle="错题本为空，交卷后客观错题会自动收录到这里" />
      <Pagination page={page} pageSize={10} total={total} onChange={setPage} />
      <ConfirmDialog
        open={!!confirmId}
        message="确定移除该错题？"
        onCancel={() => setConfirmId(null)}
        onConfirm={async () => {
          if (confirmId) {
            await remove(confirmId);
            setConfirmId(null);
            reload();
          }
        }}
      />
    </div>
  );
}
