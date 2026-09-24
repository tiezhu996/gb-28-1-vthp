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
func (f *fakeWrongBookRepo) List(_ context.Context, _ bson.M, _, _ int64) ([]*model.WrongBook, int64, error) {
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

// 构造带 1 道单选错题的已提交答卷（同时含答对的判断题与答错的填空题，均不应收录）。
func buildSubmittedRecordWithWrongObjective(student, examID primitive.ObjectID, submittedAt time.Time) *model.ExamRecord {
	return &model.ExamRecord{
		ID:          primitive.NewObjectID(),
		ExamID:      examID,
		ExamTitle:   "测试卷",
		StudentID:   student,
		StudentName: "李同学",
		Status:      constants.RecordStatusSubmitted,
		SubmittedAt: &submittedAt,
		Questions: []model.AttemptQuestion{
			{
				QuestionID:      testWrongQuestionID,
				Type:            constants.QuestionTypeSingle,
				Subject:         "数学",
				KnowledgePoints: []string{"代数"},
				Content:         "1+1=?",
				CorrectAnswer:   "B",
				UserAnswer:      "A",
				Result:          constants.AnswerResultWrong,
				Score:           5,
			},
			{
				QuestionID:    primitive.NewObjectID(),
				Type:          constants.QuestionTypeJudge,
				CorrectAnswer: "true",
				UserAnswer:    "true",
				Result:        constants.AnswerResultCorrect,
				Score:         2,
			},
			{
				QuestionID:    primitive.NewObjectID(),
				Type:          constants.QuestionTypeFill,
				CorrectAnswer: "3.14",
				UserAnswer:    "3.15",
				Result:        constants.AnswerResultUnmarked,
				Score:         3,
			},
		},
	}
}

var testWrongQuestionID = primitive.NewObjectID()

func TestCollectFromRecordFirstAndRepeat(t *testing.T) {
	questionRepo := newFakeQuestionRepo()
	questionSvc := NewQuestionService(questionRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	wrongRepo := newFakeWrongBookRepo()
	svc := NewWrongBookService(wrongRepo, questionSvc, newTestRecordSvc(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	student := primitive.NewObjectID()
	exam1 := primitive.NewObjectID()
	exam2 := primitive.NewObjectID()

	// 题库中存在该题，首次收录应带解析
	if err := questionRepo.Create(context.Background(), &model.Question{
		ID:              testWrongQuestionID,
		Type:            "single",
		Subject:         "数学",
		KnowledgePoints: []string{"代数"},
		Difficulty:      "easy",
		Content:         "1+1=?",
		Answer:          "B",
		Analysis:        "一加一等于二",
		Score:           5,
		Options: []model.QuestionOption{
			{Key: "A", Text: "1"},
			{Key: "B", Text: "2"},
		},
	}); err != nil {
		t.Fatalf("seed question: %v", err)
	}

	// ---- 首次交卷：首次收录，保留试卷、作答、解析 ----
	firstAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	rec1 := buildSubmittedRecordWithWrongObjective(student, exam1, firstAt)
	if err := svc.CollectFromRecord(context.Background(), rec1); err != nil {
		t.Fatalf("CollectFromRecord(first) error = %v", err)
	}
	entry, err := svc.repo.FindByStudentAndQuestion(context.Background(), student, testWrongQuestionID)
	if err != nil {
		t.Fatalf("entry not found: %v", err)
	}
	if entry.WrongCount != 1 {
		t.Fatalf("wrong count = %d, want 1", entry.WrongCount)
	}
	if !entry.LastWrongAt.Equal(firstAt) {
		t.Fatalf("last wrong at = %v, want %v", entry.LastWrongAt, firstAt)
	}
	if !entry.CreatedAt.Equal(firstAt) {
		t.Fatalf("created at = %v, want %v", entry.CreatedAt, firstAt)
	}
	if entry.MyAnswer != "A" || entry.ExamID != exam1 || entry.ExamRecordID != rec1.ID {
		t.Fatalf("first snapshot not kept: %+v", entry)
	}
	if entry.Analysis != "一加一等于二" {
		t.Fatalf("analysis = %q, want 首次收录保留解析", entry.Analysis)
	}
	if entry.Status != constants.WrongBookStatusActive {
		t.Fatalf("status = %s, want active", entry.Status)
	}

	// 学生把题目标记为已掌握并写下备注
	if _, err := svc.Update(context.Background(), entry.ID, student, constants.WrongBookStatusResolved, "二刷注意"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	firstCreatedAt := entry.CreatedAt

	// ---- 再次交卷答错：更新最近试卷/作答/时间，次数 +1，状态回到未掌握，备注与首次收录时间保留 ----
	secondAt := firstAt.Add(48 * time.Hour)
	rec2 := buildSubmittedRecordWithWrongObjective(student, exam2, secondAt)
	rec2.Questions[0].UserAnswer = "C"
	if err := svc.CollectFromRecord(context.Background(), rec2); err != nil {
		t.Fatalf("CollectFromRecord(second) error = %v", err)
	}
	got, err := svc.repo.FindByStudentAndQuestion(context.Background(), student, testWrongQuestionID)
	if err != nil {
		t.Fatalf("entry not found after repeat: %v", err)
	}
	if got.WrongCount != 2 {
		t.Fatalf("wrong count = %d, want 2", got.WrongCount)
	}
	if got.MyAnswer != "C" {
		t.Fatalf("my answer = %q, want C（最近一次作答）", got.MyAnswer)
	}
	if got.ExamID != exam2 || got.ExamRecordID != rec2.ID {
		t.Fatalf("exam refs not updated to latest paper: %+v", got)
	}
	if !got.LastWrongAt.Equal(secondAt) {
		t.Fatalf("last wrong at = %v, want %v", got.LastWrongAt, secondAt)
	}
	if !got.CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("首次收录时间被改动: got %v, want %v", got.CreatedAt, firstCreatedAt)
	}
	if got.Note != "二刷注意" {
		t.Fatalf("备注丢失: got %q", got.Note)
	}
	if got.Analysis != "一加一等于二" {
		t.Fatalf("首次解析丢失: got %q", got.Analysis)
	}
	if got.Status != constants.WrongBookStatusActive {
		t.Fatalf("status = %s, want active（重新回到未掌握）", got.Status)
	}
}

// 客观题答对/主观题不应被收录；同一学生只应产生一条错题。
func TestCollectFromRecordSkipsCorrectAndSubjective(t *testing.T) {
	questionSvc := NewQuestionService(newFakeQuestionRepo(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	wrongRepo := newFakeWrongBookRepo()
	svc := NewWrongBookService(wrongRepo, questionSvc, newTestRecordSvc(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	student := primitive.NewObjectID()
	rec := buildSubmittedRecordWithWrongObjective(student, primitive.NewObjectID(), time.Now())
	if err := svc.CollectFromRecord(context.Background(), rec); err != nil {
		t.Fatalf("CollectFromRecord() error = %v", err)
	}
	if err := svc.CollectFromRecord(context.Background(), rec); err != nil {
		t.Fatalf("CollectFromRecord(repeat) error = %v", err)
	}
	list, total, err := svc.List(context.Background(), student, bson.M{}, 1, 20)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("total = %d, len = %d, want 1", total, len(list))
	}
	if list[0].QuestionID != testWrongQuestionID {
		t.Fatalf("收录了非客观错题: %s", list[0].QuestionID.Hex())
	}
}
