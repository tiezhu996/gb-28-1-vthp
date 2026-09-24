package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
)

// fakeWrongBookRepo 内存版错题本仓储。
type fakeWrongBookRepo struct {
	items map[string]*model.WrongBook
}

func newFakeWrongBookRepo() *fakeWrongBookRepo {
	return &fakeWrongBookRepo{items: make(map[string]*model.WrongBook)}
}

func (f *fakeWrongBookRepo) Create(_ context.Context, w *model.WrongBook) error {
	f.items[w.ID.Hex()] = w
	return nil
}
func (f *fakeWrongBookRepo) Update(_ context.Context, w *model.WrongBook) error {
	f.items[w.ID.Hex()] = w
	return nil
}
func (f *fakeWrongBookRepo) Delete(_ context.Context, id primitive.ObjectID) error {
	delete(f.items, id.Hex())
	return nil
}
func (f *fakeWrongBookRepo) FindByID(_ context.Context, id primitive.ObjectID) (*model.WrongBook, error) {
	if w, ok := f.items[id.Hex()]; ok {
		return w, nil
	}
	return nil, repository.ErrNotFound
}
func (f *fakeWrongBookRepo) FindByStudentAndQuestion(_ context.Context, studentID, questionID primitive.ObjectID) (*model.WrongBook, error) {
	for _, w := range f.items {
		if w.StudentID == studentID && w.QuestionID == questionID {
			return w, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeWrongBookRepo) List(_ context.Context, _ bson.M, _ bson.D, _, _ int64) ([]*model.WrongBook, int64, error) {
	var out []*model.WrongBook
	for _, w := range f.items {
		out = append(out, w)
	}
	return out, int64(len(out)), nil
}

func TestWrongBookAddAndResolve(t *testing.T) {
	questionRepo := newFakeQuestionRepo()
	questionSvc := NewQuestionService(questionRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recordRepo := newFakeRecordRepo()
	recordSvc := NewExamRecordService(recordRepo, newTestExamSvc(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc := NewWrongBookService(newFakeWrongBookRepo(), questionSvc, recordSvc, slog.New(slog.NewTextHandler(io.Discard, nil)))

	student := primitive.NewObjectID()
	q, err := questionSvc.Create(context.Background(), &dto.CreateQuestionRequest{
		Type: "single", Subject: "数学", KnowledgePoints: []string{"代数"},
		Difficulty: "easy", Content: "1+1=?", Answer: "B", Score: 5,
		Options: []dto.OptionInput{{Key: "A", Text: "1"}, {Key: "B", Text: "2"}},
	}, student)
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	entry, err := svc.Add(context.Background(), student, q.ID, primitive.NilObjectID, primitive.NilObjectID, "复习")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if entry.Status != constants.WrongBookStatusActive {
		t.Fatalf("status = %s", entry.Status)
	}
	// 幂等
	if _, err := svc.Add(context.Background(), student, q.ID, primitive.NilObjectID, primitive.NilObjectID, "复习"); err != nil {
		t.Fatalf("duplicate Add() error = %v", err)
	}
	updated, err := svc.Update(context.Background(), entry.ID, student, constants.WrongBookStatusResolved, "")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Status != constants.WrongBookStatusResolved {
		t.Fatalf("status = %s", updated.Status)
	}
}

// TestSubmitAutoCollectWrongBook 交卷自动收录客观错题：
// 首次收录保留解析；再次答错改存最近试卷/作答/交卷时间，次数+1、状态回到未掌握，备注与首次收录时间保留。
func TestSubmitAutoCollectWrongBook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	questionRepo := newFakeQuestionRepo()
	questionSvc := NewQuestionService(questionRepo, logger)
	examSvc := NewExamService(newFakeExamRepo(), questionSvc, logger)
	recordSvc := NewExamRecordService(newFakeRecordRepo(), examSvc, logger)
	wbSvc := NewWrongBookService(newFakeWrongBookRepo(), questionSvc, recordSvc, logger)
	recordSvc.SetWrongBookCollector(wbSvc)

	ctx := context.Background()
	teacher := primitive.NewObjectID()
	student := primitive.NewObjectID()
	q, err := questionSvc.Create(ctx, &dto.CreateQuestionRequest{
		Type: "single", Subject: "数学", KnowledgePoints: []string{"代数"},
		Difficulty: "easy", Content: "1+1=?", Answer: "B", Score: 5, Analysis: "因为1+1=2",
		Options: []dto.OptionInput{{Key: "A", Text: "1"}, {Key: "B", Text: "2"}},
	}, teacher)
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	now := time.Now()
	exam, err := examSvc.Create(ctx, &dto.CreateExamRequest{
		Title: "自动收录测试卷", Subject: "数学", DurationMin: 30, PassScore: 60,
		StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		Questions: []dto.ExamQuestionInput{{QuestionID: q.ID.Hex()}},
	}, teacher)
	if err != nil {
		t.Fatalf("create exam: %v", err)
	}
	if _, err := examSvc.Publish(ctx, exam.ID, "t@example.com"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// 第一次考试答错 → 交卷自动收录
	rec, err := recordSvc.StartExam(ctx, exam.ID, student, "李同学")
	if err != nil {
		t.Fatalf("StartExam() error = %v", err)
	}
	if _, err := recordSvc.Submit(ctx, rec.ID, []dto.AnswerInput{{QuestionID: q.ID.Hex(), Answer: "A"}}, 0, nil, false); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	entry, err := wbSvc.repo.FindByStudentAndQuestion(ctx, student, q.ID)
	if err != nil {
		t.Fatalf("交卷后应自动收录错题: %v", err)
	}
	if entry.WrongCount != 1 {
		t.Fatalf("wrong_count = %d, want 1", entry.WrongCount)
	}
	if entry.Status != constants.WrongBookStatusActive {
		t.Fatalf("status = %s, want active", entry.Status)
	}
	if entry.Analysis != "因为1+1=2" {
		t.Fatalf("首次收录应保留试卷答案解析, got %q", entry.Analysis)
	}
	if entry.MyAnswer != "A" || entry.ExamID != exam.ID || entry.ExamRecordID != rec.ID {
		t.Fatalf("首次收录的试卷/作答不正确: %+v", entry)
	}
	if entry.LastWrongAt.IsZero() {
		t.Fatal("最近出错时间不应为空")
	}
	firstCollectedAt := entry.CreatedAt

	// 学生写备注并标记已掌握
	if _, err := wbSvc.Update(ctx, entry.ID, student, constants.WrongBookStatusResolved, "易混点"); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// 第二次考试同一题再次答错 → 次数+1、状态回到未掌握、备注与首次收录时间保留
	rec2, err := recordSvc.StartExam(ctx, exam.ID, student, "李同学")
	if err != nil {
		t.Fatalf("second StartExam() error = %v", err)
	}
	if _, err := recordSvc.Submit(ctx, rec2.ID, []dto.AnswerInput{{QuestionID: q.ID.Hex(), Answer: "C"}}, 0, nil, false); err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	entry2, err := wbSvc.repo.FindByStudentAndQuestion(ctx, student, q.ID)
	if err != nil {
		t.Fatalf("find wrong book: %v", err)
	}
	if entry2.ID != entry.ID {
		t.Fatal("再次答错不应重复收录")
	}
	if entry2.WrongCount != 2 {
		t.Fatalf("wrong_count = %d, want 2", entry2.WrongCount)
	}
	if entry2.Status != constants.WrongBookStatusActive {
		t.Fatalf("再次答错后状态应回到未掌握, got %s", entry2.Status)
	}
	if entry2.MyAnswer != "C" || entry2.ExamRecordID != rec2.ID {
		t.Fatalf("再次答错应改存最近一次作答与答卷: %+v", entry2)
	}
	if !entry2.LastWrongAt.After(entry.LastWrongAt) && !entry2.LastWrongAt.Equal(entry.LastWrongAt) {
		t.Fatal("最近出错时间应更新为最近一次交卷时间")
	}
	if entry2.Note != "易混点" {
		t.Fatalf("原备注应保留, got %q", entry2.Note)
	}
	if !entry2.CreatedAt.Equal(firstCollectedAt) {
		t.Fatal("首次收录时间不应被覆盖")
	}
	if entry2.Analysis != "因为1+1=2" {
		t.Fatalf("首次收录的解析应保留, got %q", entry2.Analysis)
	}
}
