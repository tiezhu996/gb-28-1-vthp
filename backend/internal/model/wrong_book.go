package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WrongBook 错题本实体，集合 wrong_books。
// 状态枚举：active / resolved。
type WrongBook struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	StudentID       primitive.ObjectID `bson:"student_id" json:"student_id"`
	QuestionID      primitive.ObjectID `bson:"question_id" json:"question_id"`
	ExamID          primitive.ObjectID `bson:"exam_id" json:"exam_id"`
	ExamRecordID    primitive.ObjectID `bson:"exam_record_id" json:"exam_record_id"`
	Subject         string             `bson:"subject" json:"subject"`
	KnowledgePoints []string           `bson:"knowledge_points" json:"knowledge_points"`
	QuestionContent string             `bson:"question_content" json:"question_content"`
	MyAnswer        string             `bson:"my_answer" json:"my_answer"`
	CorrectAnswer   string             `bson:"correct_answer" json:"correct_answer"`
	Analysis        string             `bson:"analysis" json:"analysis"`
	Note            string             `bson:"note" json:"note"`
	Status          string             `bson:"status" json:"status"`
	WrongCount      int                `bson:"wrong_count" json:"wrong_count"`     // 累计答错次数（首次收录为 1）
	LastWrongAt     time.Time          `bson:"last_wrong_at" json:"last_wrong_at"` // 最近一次答错（交卷）时间
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`       // 首次收录时间，再次答错也不变
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
}
