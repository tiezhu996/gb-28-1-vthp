package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// WrongBookService 错题本服务：加入/复习/移除。
type WrongBookService struct {
	repo     repository.WrongBookRepository
	question *QuestionService // 复用题库服务（加载题目快照）
	record   *ExamRecordService // 复用考试记录服务（获取学生作答）
	logger   *slog.Logger
}

// NewWrongBookService 构造错题本服务。
func NewWrongBookService(repo repository.WrongBookRepository, question *QuestionService, record *ExamRecordService, logger *slog.Logger) *WrongBookService {
	return &WrongBookService{repo: repo, question: question, record: record, logger: logger}
}

// Add 将错题加入错题本（幂等：已存在则直接返回）。
func (s *WrongBookService) Add(ctx context.Context, studentID, questionID, examID, examRecordID primitive.ObjectID, note string) (*model.WrongBook, error) {
	if existing, err := s.repo.FindByStudentAndQuestion(ctx, studentID, questionID); err == nil {
		return existing, nil
	}
	q, err := s.question.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	myAnswer := ""
	if !examRecordID.IsZero() {
		rec, err := s.record.GetByID(ctx, examRecordID)
		if err == nil {
			for i := range rec.Questions {
				if rec.Questions[i].QuestionID == questionID {
					myAnswer = rec.Questions[i].UserAnswer
					break
				}
			}
		}
	}
	now := time.Now()
	entry := &model.WrongBook{
		ID:              primitive.NewObjectID(),
		StudentID:       studentID,
		QuestionID:      questionID,
		ExamID:          examID,
		ExamRecordID:    examRecordID,
		Subject:         q.Subject,
		KnowledgePoints: q.KnowledgePoints,
		QuestionContent: q.Content,
		MyAnswer:        myAnswer,
		CorrectAnswer:   q.Answer,
		Analysis:        q.Analysis,
		Note:            note,
		Status:          constants.WrongBookStatusActive,
		WrongCount:      1,
		LastWrongAt:     now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, fmt.Errorf("wrong book service add: %w", err)
	}
	s.logger.Info(constants.LogWrongBookAdded, "student", studentID.Hex(), "question_id", questionID.Hex())
	return entry, nil
}

// CollectFromRecord 交卷后自动收录客观错题（由 ExamRecordService.Submit 回调）。
// 首次收录保留试卷答案解析；再次答错改存最近一次试卷、作答与交卷时间，
// 错误次数加一、状态回到未掌握，原先备注与首次收录时间保留。
// 收录失败不阻断交卷，仅记录告警日志。
func (s *WrongBookService) CollectFromRecord(ctx context.Context, rec *model.ExamRecord) {
	collected := 0
	for i := range rec.Questions {
		q := &rec.Questions[i]
		if !constants.IsObjectiveQuestion(q.Type) || q.Result != constants.AnswerResultWrong {
			continue
		}
		if _, err := s.upsertFromAttempt(ctx, rec, q); err != nil {
			s.logger.Warn("错题自动收录失败", "record_id", rec.ID.Hex(), "question_id", q.QuestionID.Hex(), "error", err.Error())
			continue
		}
		collected++
	}
	if collected > 0 {
		s.logger.Info(constants.LogWrongBookAutoCollect, "record_id", rec.ID.Hex(), "student", rec.StudentName, "count", collected)
	}
}

// upsertFromAttempt 按学生+题目幂等收录错题（CollectFromRecord 内部复用）。
func (s *WrongBookService) upsertFromAttempt(ctx context.Context, rec *model.ExamRecord, q *model.AttemptQuestion) (*model.WrongBook, error) {
	now := time.Now()
	lastWrongAt := now
	if rec.SubmittedAt != nil {
		lastWrongAt = *rec.SubmittedAt
	}
	existing, err := s.repo.FindByStudentAndQuestion(ctx, rec.StudentID, q.QuestionID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("wrong book service collect find: %w", err)
	}
	if existing != nil {
		// 再次答错：改存最近一次试卷、作答与交卷时间；错误次数加一；状态回到未掌握；
		// 原先备注、首次收录时间与首次收录的解析保持不变。
		existing.ExamID = rec.ExamID
		existing.ExamRecordID = rec.ID
		existing.MyAnswer = q.UserAnswer
		if existing.WrongCount < 1 {
			existing.WrongCount = 1
		}
		existing.WrongCount++
		existing.Status = constants.WrongBookStatusActive
		existing.LastWrongAt = lastWrongAt
		existing.UpdatedAt = now
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("wrong book service collect update: %w", err)
		}
		s.logger.Info(constants.LogWrongBookRepeated, "student", rec.StudentID.Hex(), "question_id", q.QuestionID.Hex(), "wrong_count", existing.WrongCount)
		return existing, nil
	}
	// 首次收录：保留试卷答案解析（题库中的解析快照）。
	question, err := s.question.GetByID(ctx, q.QuestionID)
	if err != nil {
		return nil, err
	}
	entry := &model.WrongBook{
		ID:              primitive.NewObjectID(),
		StudentID:       rec.StudentID,
		QuestionID:      q.QuestionID,
		ExamID:          rec.ExamID,
		ExamRecordID:    rec.ID,
		Subject:         q.Subject,
		KnowledgePoints: q.KnowledgePoints,
		QuestionContent: q.Content,
		MyAnswer:        q.UserAnswer,
		CorrectAnswer:   q.CorrectAnswer,
		Analysis:        question.Analysis,
		Status:          constants.WrongBookStatusActive,
		WrongCount:      1,
		LastWrongAt:     lastWrongAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		return nil, fmt.Errorf("wrong book service collect create: %w", err)
	}
	s.logger.Info(constants.LogWrongBookAdded, "student", rec.StudentID.Hex(), "question_id", q.QuestionID.Hex())
	return entry, nil
}

// Update 更新错题本（标记已掌握/修改备注）。
func (s *WrongBookService) Update(ctx context.Context, id, studentID primitive.ObjectID, status, note string) (*model.WrongBook, error) {
	entry, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeWrongBookNotFound, fmt.Sprintf("错题本模块：id=%s 的条目不存在", id.Hex()))
		}
		return nil, fmt.Errorf("wrong book service update find: %w", err)
	}
	if entry.StudentID != studentID {
		return nil, util.NewAppError(constants.CodeForbidden, constants.MsgForbidden)
	}
	if status != "" {
		if status != constants.WrongBookStatusActive && status != constants.WrongBookStatusResolved {
			return nil, util.NewAppError(constants.CodeBadRequest, fmt.Sprintf(constants.MsgValidationFailed, "status"))
		}
		entry.Status = status
	}
	if note != "" {
		entry.Note = note
	}
	entry.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, entry); err != nil {
		return nil, fmt.Errorf("wrong book service update: %w", err)
	}
	if entry.Status == constants.WrongBookStatusResolved {
		s.logger.Info(constants.LogWrongBookResolved, "student", studentID.Hex(), "question_id", entry.QuestionID.Hex())
	}
	return entry, nil
}

// Delete 移除错题。
func (s *WrongBookService) Delete(ctx context.Context, id, studentID primitive.ObjectID) error {
	entry, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeWrongBookNotFound, fmt.Sprintf("错题本模块：id=%s 的条目不存在", id.Hex()))
		}
		return fmt.Errorf("wrong book service delete find: %w", err)
	}
	if entry.StudentID != studentID {
		return util.NewAppError(constants.CodeForbidden, constants.MsgForbidden)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("wrong book service delete: %w", err)
	}
	return nil
}

// List 分页查询学生错题本（sort 由 handler 根据查询参数构造，如按错误次数倒序）。
func (s *WrongBookService) List(ctx context.Context, studentID primitive.ObjectID, filter bson.M, sort bson.D, page, pageSize int64) ([]*model.WrongBook, int64, error) {
	filter["student_id"] = studentID
	list, total, err := s.repo.List(ctx, filter, sort, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("wrong book service list: %w", err)
	}
	return list, total, nil
}

// GetByID 查询单个错题（仅本人）。
func (s *WrongBookService) GetByID(ctx context.Context, id, studentID primitive.ObjectID) (*model.WrongBook, error) {
	entry, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeWrongBookNotFound, fmt.Sprintf("错题本模块：id=%s 的条目不存在", id.Hex()))
		}
		return nil, fmt.Errorf("wrong book service get: %w", err)
	}
	if entry.StudentID != studentID {
		return nil, util.NewAppError(constants.CodeForbidden, constants.MsgForbidden)
	}
	return entry, nil
}
